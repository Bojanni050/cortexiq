# Benchmark Datasets

This directory contains benchmark datasets for the `cortexiq benchmark` command.

## Format

Datasets are JSON files with this structure (compatible with LongMemEval):

```json
{
  "name": "my-benchmark",
  "items": [
    {
      "question_id": "item-001",
      "haystack_sessions": [
        {
          "id": "s1",
          "date": "2025-01-15",
          "turns": [
            {"role": "user", "content": "I moved to Amsterdam."},
            {"role": "assistant", "content": "That's great!"}
          ]
        }
      ],
      "qa_pairs": [
        {
          "id": "q1",
          "category": "single-session",
          "question": "Where does the user live?",
          "answer": "Amsterdam",
          "question_date": "2025-02-01"
        }
      ]
    }
  ]
}
```

### Fields

| Field | Description |
|-------|-------------|
| `name` | Benchmark name (shown in reports) |
| `items[].question_id` | Unique item identifier |
| `items[].haystack_sessions[]` | Conversation sessions to ingest |
| `sessions[].id` | Session identifier within the item |
| `sessions[].date` | When the session happened (YYYY-MM-DD or RFC3339) |
| `sessions[].turns[]` | Chat turns: `{"role": "user\|assistant", "content": "..."}` |
| `items[].qa_pairs[]` | Questions to answer after ingesting |
| `qa_pairs[].id` | Unique question identifier |
| `qa_pairs[].category` | Category for breakdown (e.g. "single-session", "multi-session", "temporal") |
| `qa_pairs[].question` | The question to answer |
| `qa_pairs[].answer` | Gold-standard answer |
| `qa_pairs[].question_date` | Optional: when the question was asked (for temporal questions) |

## Categories

Common question categories:

- **single-session**: Answer is in one session
- **multi-session**: Answer requires combining info from multiple sessions
- **temporal**: Answer depends on dates/timing
- **abstention**: The system should say it doesn't know (answer is "unknown")
- **long-context**: Answer is buried in a long conversation

## Usage

```bash
# List available datasets
cortexiq benchmark list

# Run a benchmark
cortexiq benchmark run --dataset eval/benchmark/example.json

# Run with LLM judge (more accurate scoring)
cortexiq benchmark run --dataset eval/benchmark/example.json --llm-judge

# Run a subset
cortexiq benchmark run --dataset eval/benchmark/example.json --max-items 5

# Save report to file
cortexiq benchmark run --dataset eval/benchmark/example.json --output report.json
```

## Included Datasets

| Dataset | Items | Questions | Description |
|---------|-------|-----------|-------------|
| `example.json` | 3 | 12 | Small example with single-session, multi-session, and temporal questions |

## Adding Your Own

1. Create a JSON file in this directory following the format above
2. Run `cortexiq benchmark list` to verify it loads
3. Run `cortexiq benchmark run --dataset eval/benchmark/your-dataset.json` to test

## LongMemEval

To use the [LongMemEval](https://github.com/xiaowu0162/LongMemEval) benchmark:

1. Download the dataset from the LongMemEval repository
2. Convert it to the format above (or use it directly — the loader accepts both
   the wrapped `{name, items}` format and a bare array of items)
3. Run: `cortexiq benchmark run --dataset longmemeval.json --llm-judge`
