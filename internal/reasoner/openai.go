// Modifications copyright (c) 2026 Bojan van der Heide
package reasoner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/Bojanni050/cortexiq/internal/models"
	"github.com/Bojanni050/cortexiq/internal/observability"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"
)

const systemPrompt = `You are a strict information extraction engine.

Rules:
- Extract ONLY what is explicitly stated in the provided text.
- You may use language understanding to interpret what is written.
- You may NOT infer, assume, generalize, or fill in gaps from your own knowledge.
- If a field cannot be found in the text, output null for that field.
- Never guess. Never approximate. Never complete missing information.
- When in doubt, output null.
- Output ONLY valid JSON matching the provided schema.
- No explanation. No preamble. No markdown. Just the JSON object.`

const retryWarning = "Your previous response was invalid or contained invented information. Follow the rules strictly. Output ONLY valid JSON matching the provided schema."

type OpenAI struct {
	client    openai.Client
	model     string
	maxTokens int
	jsonMode  bool
}

func NewOpenAI(baseURL, apiKey, model string, maxTokens int, jsonMode bool) (*OpenAI, error) {
	if apiKey == "" {
		return nil, errors.New("reasoner: apiKey is required")
	}
	if model == "" {
		return nil, errors.New("reasoner: model is required")
	}

	client := openai.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey(apiKey),
	)

	return &OpenAI{
		client:    client,
		model:     model,
		maxTokens: maxTokens,
		jsonMode:  jsonMode,
	}, nil
}

// chat performs a single chat completion with the configured output constraints.
// max_tokens is only sent when greater than zero, so "unlimited" truly omits the
// field (sending 0 would request zero output). Native JSON output is requested
// only when jsonMode is enabled, since provider/model support varies.
func (o *OpenAI) chat(ctx context.Context, stage string, msgs []openai.ChatCompletionMessageParamUnion) (string, error) {
	params := openai.ChatCompletionNewParams{
		Model:    o.model,
		Messages: msgs,
	}
	if o.maxTokens > 0 {
		params.MaxTokens = openai.Int(int64(o.maxTokens))
	}
	if o.jsonMode {
		rf := shared.NewResponseFormatJSONObjectParam()
		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{OfJSONObject: &rf}
	}

	resp, err := o.client.Chat.Completions.New(ctx, params)
	if err != nil {
		observability.RecordReasonerCall(stage, o.model, 0, 0, false)
		return "", fmt.Errorf("chat.completions call failed: %w", err)
	}
	if len(resp.Choices) == 0 {
		observability.RecordReasonerCall(stage, o.model, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, false)
		return "", errors.New("reasoner: no response from LLM")
	}
	observability.RecordReasonerCall(stage, o.model, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, true)
	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

// --- JSON response types ---

type jsonFact struct {
	Entity   *string `json:"entity"`
	Property *string `json:"property"`
	Value    *string `json:"value"`
	Summary  *string `json:"summary"`
}

type jsonRelationship struct {
	FactID       int64  `json:"fact_id"`
	From         string `json:"from"`
	RelationType string `json:"relation_type"`
	// CanonicalType is the model's optional pick from RelationVocabulary; it is
	// validated (and may be overridden by a reviewed mapping) in CanonicalRelationType.
	CanonicalType string  `json:"canonical_type"`
	To            string  `json:"to"`
	Confidence    float32 `json:"confidence"`
}

type jsonPattern struct {
	Pattern     string  `json:"pattern"`
	Coherence   float32 `json:"coherence"`
	SourceFacts []int64 `json:"source_facts"`
	SourceRels  []int64 `json:"source_rels"`
}

type jsonContradiction struct {
	Classification string  `json:"classification"`
	Confidence     float32 `json:"confidence"`
	Explanation    string  `json:"explanation"`
}

type jsonCausalLink struct {
	CauseID    int64   `json:"cause_id"`
	EffectID   int64   `json:"effect_id"`
	Confidence float32 `json:"confidence"`
}

type jsonGoalProgress struct {
	GoalID     int64   `json:"goal_id"`
	Assessment string  `json:"assessment"`
	Note       string  `json:"note"`
	Confidence float32 `json:"confidence"`
}

type jsonFailurePattern struct {
	Type        string  `json:"type"`
	FailureID   int64   `json:"failure_id"`
	Evidence    string  `json:"evidence"`
	PatternFact string  `json:"pattern_fact"`
	Confidence  float32 `json:"confidence"`
}

type jsonHypothesisEvidence struct {
	HypothesisID  int64   `json:"hypothesis_id"`
	Verdict       string  `json:"verdict"`
	Confidence    float32 `json:"confidence"`
	Reasoning     string  `json:"reasoning"`
	NewConfidence float32 `json:"new_confidence"`
}

// --- ReasonSameFact ---

func (o *OpenAI) ReasonSameFact(ctx context.Context, existing, candidate string) (bool, error) {
	if strings.TrimSpace(existing) == "" || strings.TrimSpace(candidate) == "" {
		return false, errors.New("reasoner: both facts must not be empty")
	}

	prompt := fmt.Sprintf(`Existing fact: %s

New fact: %s

Does the New fact contain any information that is not already in the Existing fact?
- A rewording, a different phrasing or a translation of the same information is NOT new information.
- A different or additional value, date, number, name, version, reason or detail IS new information.

Output ONLY this JSON: {"same": true} when the New fact adds nothing, otherwise {"same": false}.`, existing, candidate)

	msgs := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
		openai.UserMessage(prompt),
	}

	var valErr error
	for attempt := 0; attempt < 2; attempt++ {
		output, err := o.chat(ctx, "same_fact", msgs)
		if err != nil {
			return false, err
		}
		var resp struct {
			Same *bool `json:"same"`
		}
		if err := json.Unmarshal([]byte(extractJSON(output)), &resp); err != nil || resp.Same == nil {
			valErr = errors.New("parse same-fact response")
			msgs = append(msgs, openai.SystemMessage(retryWarning))
			continue
		}
		return *resp.Same, nil
	}
	return false, valErr
}

// maxFactsPerCall bounds how many facts one synthesis call may return, which also
// bounds the output tokens (and cost) per call.
const maxFactsPerCall = 6

// structuredPrompt builds the fact-synthesis prompt. subject, when set, is context
// for naming the project/area only; facts must still come from the Events.
func structuredPrompt(texts []string, subject string) string {
	eventsList := strings.Join(texts, "\n- ")

	subjectBlock := ""
	if s := strings.TrimSpace(subject); s != "" {
		subjectBlock = fmt.Sprintf(`
Subject context (what the Events belong to): %s
Use it ONLY to name the project or area in the summaries. Never take facts from it.
`, s)
	}

	return fmt.Sprintf(`Given these events, extract the durable facts they state.
%s
Events:
- %s

Output ONLY a JSON array. Each element is one fact:
[{"entity": "string or null", "property": "string or null", "value": "string or null", "summary": "string"}]

Rules:
- Output ONE fact for EACH distinct claim or topic in the Events. Two topics in the Events means two facts. Never merge separate topics into one fact, and never drop one.
- Do NOT split a single claim into pieces, and do not repeat the same information in two facts. At most %d facts.
- Every summary MUST be understandable on its own, without the Events and without knowing the conversation: name the project, product or subject explicitly (e.g. "CortexIQ", "Chronicle-Gaia"). Never use "the system", "the plugin", "the app", "it" or "this" as the subject.
- If the Events contain no durable information worth remembering (small talk, a status update without outcome, an action without subject or result, a bare question), output an empty array: []
- Every summary MUST be specific and detailed. Do NOT echo, quote, or closely paraphrase a single Event sentence; rewrite in your own words.
- Remove first-person markers ("I", "my", "we") — state facts about subjects, not the narrator.
- All values MUST come ONLY from the Events. Do not add outside details.
- If a field is not explicitly stated in the Events, use null for entity, property or value.
- PRESERVE specifics: proper nouns, library names, version numbers, file paths, commands, ports, configuration names, technical terms, and quantities.
- Do NOT replace specifics with vague words like "various", "components", "functionalities", "features", "certain".
- The entity should be the most specific subject (e.g., "pgxpool" not "the system").

BAD example (vague — DO NOT do this):
[{"entity": "CortexIQ", "property": "components", "value": "various functionalities", "summary": "CortexIQ comprises various components for different functionalities."}]

BAD example (verbatim copy — DO NOT do this):
[{"entity": "I", "property": "currently_testing", "value": "the CortexIQ memory system", "summary": "I am currently testing the CortexIQ memory system."}]

BAD example (not self-contained — DO NOT do this):
[{"entity": "plugin", "property": "option", "value": "pastReference", "summary": "The plugin gained a pastReference option that enables recall."}]

GOOD example (two topics in the Events become two specific facts):
[{"entity": "CortexIQ", "property": "database", "value": "PostgreSQL with pgvector", "summary": "CortexIQ uses PostgreSQL with pgvector extension for vector storage and HNSW indexes."},
 {"entity": "CortexIQ", "property": "consolidation interval", "value": "5 minutes", "summary": "CortexIQ runs background consolidation every 5 minutes with at most 100 episodes per run."}]`, subjectBlock, eventsList, maxFactsPerCall)
}

// parseStructuredFacts reads the model output into facts. It accepts the requested
// JSON array and, leniently, a single JSON object (the old format, which models
// sometimes still produce). Facts without a summary are dropped (that is how a model
// says "nothing durable here"), identical summaries are collapsed, and the result is
// capped at maxFactsPerCall.
func parseStructuredFacts(output string) ([]*StructuredFact, error) {
	raw := extractJSON(output)

	var jfs []jsonFact
	switch {
	case strings.HasPrefix(raw, "["):
		if err := json.Unmarshal([]byte(raw), &jfs); err != nil {
			return nil, fmt.Errorf("parse json: %w", err)
		}
	case strings.HasPrefix(raw, "{"):
		var one jsonFact
		if err := json.Unmarshal([]byte(raw), &one); err != nil {
			return nil, fmt.Errorf("parse json: %w", err)
		}
		jfs = []jsonFact{one}
	default:
		return nil, errors.New("parse json: no JSON array or object in the response")
	}

	var facts []*StructuredFact
	seen := map[string]bool{}
	for _, jf := range jfs {
		sf := &StructuredFact{
			Entity:   ptrStr(jf.Entity),
			Property: ptrStr(jf.Property),
			Value:    ptrStr(jf.Value),
			Summary:  strings.TrimSpace(ptrStr(jf.Summary)),
		}
		key := strings.ToLower(sf.Summary)
		if sf.Summary == "" || seen[key] {
			continue
		}
		seen[key] = true
		facts = append(facts, sf)
		if len(facts) == maxFactsPerCall {
			break
		}
	}
	return facts, nil
}

// --- ReasonStructured ---

func (o *OpenAI) ReasonStructured(ctx context.Context, texts []string, subject string) ([]*StructuredFact, error) {
	if len(texts) == 0 {
		return nil, errors.New("reasoner: texts must not be empty")
	}

	msgs := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
		openai.UserMessage(structuredPrompt(texts, subject)),
	}

	const attempts = 2
	var valErr error
	for attempt := 0; attempt < attempts; attempt++ {
		output, err := o.chat(ctx, "structured", msgs)
		if err != nil {
			return nil, err
		}

		facts, err := parseStructuredFacts(output)
		if err != nil {
			valErr = err
			msgs = append(msgs, openai.SystemMessage(retryWarning))
			continue
		}

		// Grounding: no fact may be a (near-)verbatim copy of a source text. On the
		// last attempt keep the facts that pass instead of losing all of them.
		var good []*StructuredFact
		var groundErr error
		for _, sf := range facts {
			if err := validateFactGrounding(sf, texts); err != nil {
				groundErr = err
				continue
			}
			good = append(good, sf)
		}
		if groundErr == nil {
			return good, nil
		}
		if attempt == attempts-1 && len(good) > 0 {
			return good, nil
		}
		valErr = fmt.Errorf("grounding validation: %w", groundErr)
		msgs = append(msgs, openai.SystemMessage(retryWarning+" "+groundErr.Error()))
	}
	return nil, valErr
}

// --- ReasonRelationshipsBatch ---

func (o *OpenAI) ReasonRelationshipsBatch(ctx context.Context, facts []models.Fact) (map[int64][]*StructuredRelationship, error) {
	contents := make(map[int64]string, len(facts))
	var list strings.Builder
	for _, f := range facts {
		if f.Content == "" {
			continue
		}
		contents[f.ID] = f.Content
		fmt.Fprintf(&list, "[%d] %s\n", f.ID, f.Content)
	}
	if len(contents) == 0 {
		return nil, errors.New("reasoner: facts must not be empty")
	}

	prompt := fmt.Sprintf(`Given these facts, extract all directly stated relationships. Each fact starts with its id in square brackets.

Facts:
%s
Output ONLY a JSON array of objects:
[{"fact_id": 0, "from": "subject entity", "relation_type": "lowercase_type", "canonical_type": "vocabulary_type_or_null", "to": "object entity", "confidence": 0.0}]

Rules:
- "fact_id" must be the id of the fact the relationship is stated in. Treat every fact independently; never combine facts.
- Only extract relationships DIRECTLY stated in that fact.
- Do NOT infer transitive or implied relationships (e.g., if "Alice works at TechCorp in Paris", do NOT output Alice --located_in--> Paris).
- "from" and "to" must be entity names mentioned verbatim in that fact.
- relation_type must be a simple lowercase identifier (e.g., works_at, manages, knows). Prefer one of the vocabulary words below when it says exactly the same thing.
- "canonical_type" must be one of these words, or null: %s.
  Use it ONLY when it means the same as the relationship, in the same direction (from -> to), and the relationship is not negated. If the relationship is negated ("does not use"), reversed ("used by"), or no word fits exactly, use null. Never guess and never invent words.
- confidence must be between 0.7 and 1.0.
- If no relationships are explicitly stated in any fact, output: []`, list.String(), relationVocabularyList())

	msgs := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
		openai.UserMessage(prompt),
	}

	var valErr error
	for attempt := 0; attempt < 2; attempt++ {
		output, err := o.chat(ctx, "relationships", msgs)
		if err != nil {
			return nil, err
		}

		var jrels []jsonRelationship
		if err := json.Unmarshal([]byte(extractJSON(output)), &jrels); err != nil {
			valErr = fmt.Errorf("parse json: %w", err)
			msgs = append(msgs, openai.SystemMessage(retryWarning))
			continue
		}
		return groundRelationships(jrels, contents), nil
	}
	return nil, valErr
}

// groundRelationships keeps only well-formed relationships that are attributed to a
// known fact and whose entities appear verbatim in that fact's content. Anything
// else is dropped individually, so one bad item does not cost the whole batch.
func groundRelationships(jrels []jsonRelationship, contents map[int64]string) map[int64][]*StructuredRelationship {
	out := make(map[int64][]*StructuredRelationship)
	for _, jr := range jrels {
		content, ok := contents[jr.FactID]
		if !ok || jr.From == "" || jr.RelationType == "" || jr.To == "" {
			continue
		}
		if !stringsContains(content, jr.From) || !stringsContains(content, jr.To) {
			continue
		}
		conf := jr.Confidence
		if conf < 0.7 {
			conf = 0.7
		}
		if conf > 1.0 {
			conf = 1.0
		}
		out[jr.FactID] = append(out[jr.FactID], &StructuredRelationship{
			FromEntity:    jr.From,
			RelationType:  jr.RelationType,
			CanonicalType: CanonicalRelationType(jr.RelationType, jr.CanonicalType),
			ToEntity:      jr.To,
			Confidence:    conf,
		})
	}
	return out
}

// --- ReasonPatterns ---

func (o *OpenAI) ReasonPatterns(ctx context.Context, facts []models.Fact, relationships []models.Relationship) ([]*StructuredPattern, error) {
	if len(facts) == 0 {
		return nil, nil
	}

	var factLines []string
	for _, f := range facts {
		factLines = append(factLines, fmt.Sprintf("[Fact %d] %s (confidence: %.2f)", f.ID, f.Content, f.Confidence))
	}

	var relLines []string
	for _, r := range relationships {
		relLines = append(relLines, fmt.Sprintf("[Rel %d] %s --%s--> %s (confidence: %.2f)", r.ID, r.FromEntity, r.RelationType, r.ToEntity, r.Confidence))
	}

	prompt := fmt.Sprintf(`Given these facts and relationships, extract patterns.

Facts:
%s

Relationships:
%s

Output ONLY a JSON array of objects:
[{"pattern": "string", "coherence": 0.0, "source_facts": [1,2], "source_rels": [3,4]}]

Rules:
- A pattern MUST have at least 2 items in source_facts OR at least 2 items in source_rels. Single-source patterns are NOT allowed.
- source_facts IDs must be from the Facts list above.
- source_rels IDs must be from the Relationships list above.
- Do not generalize beyond the evidence. If no pattern meets this threshold, output: []
- The pattern statement must be directly supported by the referenced sources.`, strings.Join(factLines, "\n"), strings.Join(relLines, "\n"))

	msgs := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
		openai.UserMessage(prompt),
	}

	factIDs := make(map[int64]bool)
	for _, f := range facts {
		factIDs[f.ID] = true
	}
	relIDs := make(map[int64]bool)
	for _, r := range relationships {
		relIDs[r.ID] = true
	}

	var result []*StructuredPattern
	var valErr error

	for attempt := 0; attempt < 2; attempt++ {
		output, err := o.chat(ctx, "patterns", msgs)
		if err != nil {
			return nil, err
		}
		raw := extractJSON(output)

		var jpats []jsonPattern
		if err := json.Unmarshal([]byte(raw), &jpats); err != nil {
			valErr = fmt.Errorf("parse json: %w", err)
			msgs = append(msgs, openai.SystemMessage(retryWarning))
			continue
		}

		var validated []*StructuredPattern
		validationFailed := false

		for _, jp := range jpats {
			if jp.Pattern == "" {
				continue
			}

			validFacts, badFacts := filterIDs(jp.SourceFacts, factIDs)
			validRels, badRels := filterIDs(jp.SourceRels, relIDs)

			if badFacts || badRels {
				validationFailed = true
				break
			}

			if len(validFacts)+len(validRels) < 2 {
				continue
			}

			coherence := jp.Coherence
			if coherence <= 0 || coherence > 1.0 {
				coherence = 0.5
			}

			validated = append(validated, &StructuredPattern{
				Content:        jp.Pattern,
				CoherenceScore: coherence,
				SourceFactIDs:  validFacts,
				SourceRelIDs:   validRels,
			})
		}

		if validationFailed {
			valErr = errors.New("pattern references non-existent source IDs")
			msgs = append(msgs, openai.SystemMessage(retryWarning+" Source IDs must be from the provided lists only."))
			continue
		}

		result = validated
		valErr = nil
		break
	}

	if valErr != nil {
		return nil, valErr
	}

	return result, nil
}

// --- ReasonContradiction ---

func (o *OpenAI) ReasonContradiction(ctx context.Context, entity, property, oldValue, newValue string) (*ContradictionResult, error) {
	if entity == "" || property == "" || oldValue == "" || newValue == "" {
		return nil, errors.New("reasoner: entity, property, oldValue, and newValue are required")
	}

	prompt := fmt.Sprintf(`Given two facts about the same entity and property, classify their relationship.

Entity: %s
Property: %s
Old value: %s
New value: %s

Output ONLY this exact JSON structure:
{"classification": "replacement|contradiction|compatible", "confidence": 0.0, "explanation": "string"}

Rules:
- "replacement": The new value replaces the old one (e.g., changed address, updated title, new phone number). The old value is no longer true.
- "contradiction": Both values cannot be true simultaneously, but it is unclear which is correct. Requires human resolution.
- "compatible": Both values can be true at the same time (e.g., multiple roles, parallel attributes). No conflict.
- confidence must be between 0.0 and 1.0.
- explanation must be a brief sentence justifying the classification.`, entity, property, oldValue, newValue)

	msgs := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
		openai.UserMessage(prompt),
	}

	for attempt := 0; attempt < 2; attempt++ {
		output, err := o.chat(ctx, "contradiction", msgs)
		if err != nil {
			return nil, err
		}
		raw := extractJSON(output)

		var jc jsonContradiction
		if err := json.Unmarshal([]byte(raw), &jc); err != nil {
			msgs = append(msgs, openai.SystemMessage(retryWarning))
			continue
		}

		class := ContradictionClassification(jc.Classification)
		switch class {
		case ClassificationReplacement, ClassificationContradiction, ClassificationCompatible:
		default:
			msgs = append(msgs, openai.SystemMessage(retryWarning+" Classification must be one of: replacement, contradiction, compatible."))
			continue
		}

		conf := jc.Confidence
		if conf < 0 {
			conf = 0
		}
		if conf > 1 {
			conf = 1
		}

		return &ContradictionResult{
			Classification: class,
			Confidence:     conf,
			Explanation:    jc.Explanation,
		}, nil
	}

	return nil, errors.New("reasoner: failed to get valid contradiction classification after retries")
}

// --- ReasonCausalLinks ---

func (o *OpenAI) ReasonCausalLinks(ctx context.Context, facts []models.Fact) ([]*StructuredCausalLink, error) {
	if len(facts) < 2 {
		return nil, nil
	}

	var factLines []string
	for _, f := range facts {
		factLines = append(factLines, fmt.Sprintf("[Fact %d] %s", f.ID, f.Content))
	}

	prompt := fmt.Sprintf(`Given these facts, identify cause-effect relationships between them.

Facts:
%s

Output ONLY a JSON array of objects:
[{"cause_id": 1, "effect_id": 2, "confidence": 0.9}]

Rules:
- cause_id and effect_id must be fact IDs from the list above.
- Only identify relationships where one fact DIRECTLY caused or led to another.
- Do NOT infer transitive or indirect causation.
- A fact can be both a cause and an effect of different facts.
- cause_id and effect_id must be different (no self-loops).
- confidence must be between 0.5 and 1.0.
- If no causal relationships are evident, output: []`, strings.Join(factLines, "\n"))

	msgs := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
		openai.UserMessage(prompt),
	}

	factIDs := make(map[int64]bool)
	for _, f := range facts {
		factIDs[f.ID] = true
	}

	var result []*StructuredCausalLink
	var valErr error

	for attempt := 0; attempt < 2; attempt++ {
		output, err := o.chat(ctx, "causal_links", msgs)
		if err != nil {
			return nil, err
		}
		raw := extractJSON(output)

		var jlinks []jsonCausalLink
		if err := json.Unmarshal([]byte(raw), &jlinks); err != nil {
			valErr = fmt.Errorf("parse json: %w", err)
			msgs = append(msgs, openai.SystemMessage(retryWarning))
			continue
		}

		var validated []*StructuredCausalLink
		validationFailed := false

		for _, jl := range jlinks {
			if !factIDs[jl.CauseID] || !factIDs[jl.EffectID] {
				validationFailed = true
				break
			}
			if jl.CauseID == jl.EffectID {
				continue
			}

			conf := jl.Confidence
			if conf < 0.5 {
				conf = 0.5
			}
			if conf > 1.0 {
				conf = 1.0
			}

			validated = append(validated, &StructuredCausalLink{
				CauseFactID:  jl.CauseID,
				EffectFactID: jl.EffectID,
				Confidence:   conf,
			})
		}

		if validationFailed {
			valErr = errors.New("causal link references non-existent fact ID")
			msgs = append(msgs, openai.SystemMessage(retryWarning+" Fact IDs must be from the provided list only."))
			continue
		}

		result = validated
		valErr = nil
		break
	}

	if valErr != nil {
		return nil, valErr
	}

	return result, nil
}

// --- ReasonGoalProgress ---

func (o *OpenAI) ReasonGoalProgress(ctx context.Context, goals []models.Goal, facts []models.Fact) ([]*GoalProgressAssessment, error) {
	if len(goals) == 0 || len(facts) == 0 {
		return nil, nil
	}

	var goalLines []string
	for _, g := range goals {
		goalLines = append(goalLines, fmt.Sprintf("[Goal %d] %s", g.ID, g.Content))
	}

	var factLines []string
	for _, f := range facts {
		factLines = append(factLines, fmt.Sprintf("[Fact %d] %s", f.ID, f.Content))
	}

	prompt := fmt.Sprintf(`Given these active goals and recent facts, assess whether each fact indicates progress toward, completion of, or contradiction of any goal.

Active Goals:
%s

Recent Facts:
%s

Output ONLY a JSON array of objects:
[{"goal_id": 1, "assessment": "progress|suggested_complete|contradicted|irrelevant", "note": "brief explanation", "confidence": 0.9}]

Rules:
- goal_id must be a goal ID from the list above.
- Use "progress" if the fact indicates partial progress toward the goal.
- Use "suggested_complete" only if the fact strongly indicates the goal is fully achieved.
- Use "contradicted" if the fact contradicts or undermines the goal.
- Use "irrelevant" if the fact has no bearing on the goal.
- Only output assessments for goals where the facts are relevant. If no goals are affected, output: []
- confidence must be between 0.5 and 1.0.`, strings.Join(goalLines, "\n"), strings.Join(factLines, "\n"))

	msgs := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
		openai.UserMessage(prompt),
	}

	goalIDs := make(map[int64]bool)
	for _, g := range goals {
		goalIDs[g.ID] = true
	}

	var result []*GoalProgressAssessment
	var valErr error

	for attempt := 0; attempt < 2; attempt++ {
		output, err := o.chat(ctx, "goal_progress", msgs)
		if err != nil {
			return nil, err
		}
		raw := extractJSON(output)

		var jassess []jsonGoalProgress
		if err := json.Unmarshal([]byte(raw), &jassess); err != nil {
			valErr = fmt.Errorf("parse json: %w", err)
			msgs = append(msgs, openai.SystemMessage(retryWarning))
			continue
		}

		var validated []*GoalProgressAssessment
		validationFailed := false

		for _, ja := range jassess {
			if !goalIDs[ja.GoalID] {
				validationFailed = true
				break
			}

			assess := ja.Assessment
			switch assess {
			case "progress", "suggested_complete", "contradicted", "irrelevant":
			default:
				assess = "irrelevant"
			}

			if assess == "irrelevant" {
				continue
			}

			conf := ja.Confidence
			if conf < 0.5 {
				conf = 0.5
			}
			if conf > 1.0 {
				conf = 1.0
			}

			validated = append(validated, &GoalProgressAssessment{
				GoalID:     ja.GoalID,
				Assessment: assess,
				Note:       ja.Note,
				Confidence: conf,
			})
		}

		if validationFailed {
			valErr = errors.New("goal progress assessment references non-existent goal ID")
			msgs = append(msgs, openai.SystemMessage(retryWarning+" Goal IDs must be from the provided list only."))
			continue
		}

		result = validated
		valErr = nil
		break
	}

	if valErr != nil {
		return nil, valErr
	}

	return result, nil
}

// --- ReasonFailurePatterns ---

func (o *OpenAI) ReasonFailurePatterns(ctx context.Context, failures []models.Failure, evidence []string) ([]*FailurePatternResult, error) {
	if len(failures) == 0 || len(evidence) == 0 {
		return nil, nil
	}

	var failureLines []string
	for _, f := range failures {
		failureLines = append(failureLines, fmt.Sprintf("[Failure %d] What: %s | Why: %s | Lesson: %s", f.ID, f.Content, f.Reason, f.Lesson))
	}

	prompt := fmt.Sprintf(`Given these past failures and recent evidence, detect whether any failure is being repeated, and extract higher-order failure patterns.

Past Failures:
%s

Recent Evidence:
- %s

Output ONLY a JSON array of objects:
[{"type": "repetition", "failure_id": 1, "evidence": "what evidence suggests the repeat", "pattern_fact": "", "confidence": 0.9}]

Rules for repetition detection:
- failure_id must be a failure ID from the list above.
- Only flag a repetition if the recent evidence clearly matches the same type of mistake.
- The evidence field must explain what in the recent text matches the failure pattern.

Rules for pattern extraction (higher-order):
- If you notice a recurring theme across multiple failures, output a pattern object:
  {"type": "pattern", "failure_id": 0, "evidence": "", "pattern_fact": "agent consistently fails at X type of task", "confidence": 0.8}
- Pattern facts must be general enough to cover multiple failures but specific enough to be actionable.
- If no repetitions or patterns are found, output: []
- confidence must be between 0.5 and 1.0.`, strings.Join(failureLines, "\n"), strings.Join(evidence, "\n- "))

	msgs := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
		openai.UserMessage(prompt),
	}

	failureIDs := make(map[int64]bool)
	for _, f := range failures {
		failureIDs[f.ID] = true
	}

	var result []*FailurePatternResult
	var valErr error

	for attempt := 0; attempt < 2; attempt++ {
		output, err := o.chat(ctx, "failure_patterns", msgs)
		if err != nil {
			return nil, err
		}
		raw := extractJSON(output)

		var jpatterns []jsonFailurePattern
		if err := json.Unmarshal([]byte(raw), &jpatterns); err != nil {
			valErr = fmt.Errorf("parse json: %w", err)
			msgs = append(msgs, openai.SystemMessage(retryWarning))
			continue
		}

		var validated []*FailurePatternResult
		validationFailed := false

		for _, jp := range jpatterns {
			switch jp.Type {
			case "repetition":
				if !failureIDs[jp.FailureID] {
					validationFailed = true
					break
				}
				if jp.Evidence == "" {
					continue
				}
				conf := jp.Confidence
				if conf < 0.5 {
					conf = 0.5
				}
				if conf > 1.0 {
					conf = 1.0
				}
				validated = append(validated, &FailurePatternResult{
					Type:       "repetition",
					FailureID:  jp.FailureID,
					Evidence:   jp.Evidence,
					Confidence: conf,
				})
			case "pattern":
				if jp.PatternFact == "" {
					continue
				}
				conf := jp.Confidence
				if conf < 0.5 {
					conf = 0.5
				}
				if conf > 1.0 {
					conf = 1.0
				}
				validated = append(validated, &FailurePatternResult{
					Type:        "pattern",
					PatternFact: jp.PatternFact,
					Confidence:  conf,
				})
			default:
				continue
			}
		}

		if validationFailed {
			valErr = errors.New("failure pattern references non-existent failure ID")
			msgs = append(msgs, openai.SystemMessage(retryWarning+" Failure IDs must be from the provided list only."))
			continue
		}

		result = validated
		valErr = nil
		break
	}

	if valErr != nil {
		return nil, valErr
	}

	return result, nil
}

// --- ReasonHypothesisEvidence ---

func (o *OpenAI) ReasonHypothesisEvidence(ctx context.Context, hypotheses []models.Hypothesis, facts []models.Fact) ([]*HypothesisEvidenceResult, error) {
	if len(hypotheses) == 0 || len(facts) == 0 {
		return nil, nil
	}

	var hypLines []string
	for _, h := range hypotheses {
		hypLines = append(hypLines, fmt.Sprintf("[Hypothesis %d] (status: %s, confidence: %.2f) %s", h.ID, h.Status, h.Confidence, h.Content))
	}

	var factLines []string
	for _, f := range facts {
		factLines = append(factLines, fmt.Sprintf("[Fact %d] %s", f.ID, f.Content))
	}

	prompt := fmt.Sprintf(`Given these open hypotheses and recent facts, assess whether each fact supports, weakens, or contradicts any hypothesis.

Open Hypotheses:
%s

Recent Facts:
%s

Output ONLY a JSON array of objects:
[{"hypothesis_id": 1, "verdict": "supports|weakens|contradicts|irrelevant", "confidence": 0.9, "reasoning": "brief explanation", "new_confidence": 0.7}]

Rules:
- hypothesis_id must be a hypothesis ID from the list above.
- Use "supports" if the fact provides evidence confirming or reinforcing the hypothesis.
- Use "weakens" if the fact provides evidence against but does not conclusively disprove the hypothesis.
- Use "contradicts" if the fact conclusively disproves the hypothesis.
- Use "irrelevant" if the fact has no bearing on the hypothesis.
- new_confidence is the suggested updated confidence for the hypothesis based on this evidence.
  - If supports: new_confidence should be higher than current confidence.
  - If weakens: new_confidence should be lower than current confidence.
  - If contradicts: new_confidence should be significantly lower (below 0.3).
- Only output assessments for hypotheses where the facts are relevant. If none are affected, output: []
- confidence must be between 0.5 and 1.0.
- new_confidence must be between 0.0 and 1.0.`, strings.Join(hypLines, "\n"), strings.Join(factLines, "\n"))

	msgs := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
		openai.UserMessage(prompt),
	}

	hypIDs := make(map[int64]bool)
	for _, h := range hypotheses {
		hypIDs[h.ID] = true
	}

	var result []*HypothesisEvidenceResult
	var valErr error

	for attempt := 0; attempt < 2; attempt++ {
		output, err := o.chat(ctx, "hypothesis_evidence", msgs)
		if err != nil {
			return nil, err
		}
		raw := extractJSON(output)

		var jevidence []jsonHypothesisEvidence
		if err := json.Unmarshal([]byte(raw), &jevidence); err != nil {
			valErr = fmt.Errorf("parse json: %w", err)
			msgs = append(msgs, openai.SystemMessage(retryWarning))
			continue
		}

		var validated []*HypothesisEvidenceResult
		validationFailed := false

		for _, je := range jevidence {
			if !hypIDs[je.HypothesisID] {
				validationFailed = true
				break
			}

			verdict := je.Verdict
			switch verdict {
			case "supports", "weakens", "contradicts", "irrelevant":
			default:
				verdict = "irrelevant"
			}

			if verdict == "irrelevant" {
				continue
			}

			conf := je.Confidence
			if conf < 0.5 {
				conf = 0.5
			}
			if conf > 1.0 {
				conf = 1.0
			}

			newConf := je.NewConfidence
			if newConf < 0 {
				newConf = 0
			}
			if newConf > 1 {
				newConf = 1
			}

			validated = append(validated, &HypothesisEvidenceResult{
				HypothesisID:  je.HypothesisID,
				Verdict:       verdict,
				Confidence:    conf,
				Reasoning:     je.Reasoning,
				NewConfidence: newConf,
			})
		}

		if validationFailed {
			valErr = errors.New("hypothesis evidence references non-existent hypothesis ID")
			msgs = append(msgs, openai.SystemMessage(retryWarning+" Hypothesis IDs must be from the provided list only."))
			continue
		}

		result = validated
		valErr = nil
		break
	}

	if valErr != nil {
		return nil, valErr
	}

	return result, nil
}

// --- Helpers ---

func extractJSON(s string) string {
	s = strings.TrimSpace(s)

	trimmed := strings.TrimPrefix(s, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)

	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return trimmed
	}

	return s
}

func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func validateFactGrounding(sf *StructuredFact, texts []string) error {
	if sf.Summary == "" {
		return nil
	}

	// Anti-verbatim check: summary must not be a near-verbatim copy of any single source text
	summaryLower := strings.ToLower(sf.Summary)
	summaryWords := tokenize(summaryLower)
	if len(summaryWords) == 0 {
		return nil
	}

	for _, text := range texts {
		textLower := strings.ToLower(text)
		if strings.Contains(textLower, summaryLower) {
			return fmt.Errorf("summary is a verbatim copy of source text")
		}
		textWords := tokenize(textLower)
		if len(textWords) == 0 {
			continue
		}
		matches := 0
		for w := range summaryWords {
			if textWords[w] {
				matches++
			}
		}
		if float32(matches)/float32(len(summaryWords)) > 0.80 {
			return fmt.Errorf("summary is too close to a verbatim copy of source text")
		}
	}

	return nil
}

func tokenize(s string) map[string]bool {
	words := make(map[string]bool)
	var buf strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			buf.WriteRune(unicode.ToLower(r))
		} else {
			if buf.Len() >= 3 {
				words[buf.String()] = true
			}
			buf.Reset()
		}
	}
	if buf.Len() >= 3 {
		words[buf.String()] = true
	}
	return words
}

func stringsContains(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

func filterIDs(ids []int64, valid map[int64]bool) (filtered []int64, hasInvalid bool) {
	for _, id := range ids {
		if valid[id] {
			filtered = append(filtered, id)
		} else {
			hasInvalid = true
		}
	}
	return
}
