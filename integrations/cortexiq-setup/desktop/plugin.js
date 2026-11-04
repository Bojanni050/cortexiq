/* CortexIQ Setup — desktop-plugin (ESM, @hermes/plugin-sdk).
 *
 * Read-only surface: installatiechecklist + live verbinding via GET /status.
 * Mutaties (config, token, activeren, namespaces) bewust NIET via de desktop:
 * het bewezen plugin-Optional SDK-contract stempelt ctx.rest op GET; de
 * mutaties via SDK-fetchJSON lopen op het dashboard. Najaarly tips verwijzen
 * de gebruiker daarheen.
 */
import {
  Badge,
  Button,
  EmptyState,
  ROUTES_AREA,
  Separator,
  SIDEBAR_NAV_AREA,
  PALETTE_AREA,
  host,
  usePluginI18n,
  useQuery
} from '@hermes/plugin-sdk'
import { Fragment, jsx, jsxs } from 'react/jsx-runtime'

const VERSION = '0.1.0'
const PLUGIN_ID = 'cortexiq-setup'
const PLUGIN_NAME = 'CortexIQ Setup'
const PLUGIN_ROUTE = '/cortexiq'
const MUTED = { color: 'var(--ui-text-secondary)' }
const BORDER = { borderColor: 'var(--ui-stroke-secondary)' }

function Card({ className = '', children }) {
  return jsx('section', {
    className: `rounded-md border ${className}`,
    style: BORDER,
    children
  })
}

function CardHeader({ className = '', children }) {
  return jsx('header', { className: `p-4 ${className}`, children })
}

function CardContent({ className = '', children }) {
  return jsx('div', { className: `px-4 pb-4 ${className}`, children })
}

function CardTitle({ className = '', children }) {
  return jsx('h4', { className: `font-semibold ${className}`, children })
}

const bundles = {
  nl: {
    title: 'CortexIQ',
    subtitle: 'Installatie- en gezondheidsstatus van de CortexIQ geheugen-provider voor dit profiel.',
    refresh: 'Verversen',
    steps: 'Installatiestatus',
    server: 'CortexIQ-server',
    namespaces: 'Namespaces',
    configPath: 'Config-bestand',
    ingestNs: 'Ingest-namespace',
    provider: 'memory.provider',
    configuredHere: 'Gebruik de dashboard-pagina voor configuratie, token en activering.',
    loading: 'Setup-status laden…',
    failed: 'Kon de setup-status niet laden',
    healthLabel: 'verbinding'
  },
  en: {
    title: 'CortexIQ',
    subtitle: 'Install and health state of the CortexIQ memory provider for this profile.',
    refresh: 'Refresh',
    steps: 'Install status',
    server: 'CortexIQ server',
    namespaces: 'Namespaces',
    configPath: 'Config file',
    ingestNs: 'Ingest namespace',
    provider: 'memory.provider',
    settings: 'Provider settings',
    configuredHere: 'Use the dashboard page for configuration, token and activation.',
    loading: 'Loading setup state…',
    failed: 'Unable to load setup state',
    notConfigured: 'CortexIQ is not fully set up yet.',
    missing: 'Missing',
    allPresent: 'All required namespaces exist',
    tokenMissing: 'API token missing',
    healthLabel: 'connection'
  }
}

function StepRow({ step }) {
  return jsxs('div', {
    className: 'flex items-baseline gap-2 text-sm',
    children: [
      jsx(Badge, { variant: step.ok ? 'outline' : 'secondary', children: step.ok ? '✓' : '✗' }),
      jsx('span', { className: 'font-medium', children: step.label }),
      jsx('span', { style: MUTED, className: 'text-xs', children: step.detail || '' })
    ]
  })
}

function KeyValue({ label, value }) {
  return jsxs('div', {
    className: 'flex gap-3 text-sm',
    children: [
      jsx('span', { className: 'w-40 shrink-0 font-mono text-xs', style: MUTED, children: label }),
      jsx('span', { className: 'min-w-0 break-all', children: value })
    ]
  })
}

export function createSetupApi(ctx) {
  return {
    getStatus: () => ctx.rest(`/api/plugins/${PLUGIN_ID}/status`, { timeoutMs: 30_000 })
  }
}

function StatusCard({ status, t }) {
  const steps = (status && status.steps) || []
  const config = status && status.config
  const server = status && status.server
  const nsStep = steps.find(step => step && step.id === 'namespaces_ready')
  return jsxs(Fragment, {
    children: [
      jsx(Card, {
        className: 'w-full',
        children: jsxs(CardContent, { className: 'p-4', children: [
          jsx(CardTitle, { children: t('steps') }),
          jsx('div', { className: 'mt-3 space-y-2', children: steps.map(step => jsx(StepRow, { step }, step.id)) })
        ] })
      }),
      jsx(Card, {
        className: 'w-full',
        children: jsxs(CardContent, { className: 'p-4', children: [
          jsx(CardTitle, { children: t('server') }),
          jsx('div', { className: 'mt-3 space-y-2', children: [
            jsx(KeyValue, { label: 'api_url', value: (config && config.api_url) || '—' }),
            jsx(KeyValue, { label: t('healthLabel'), value: server && server.live ? `live (${server.latency_ms} ms)` : (server && server.error) || '—' }),
            jsx(KeyValue, { label: t('provider'), value: status && status.provider_selected && status.provider_selected.is_cortexiq ? 'cortexiq' : ((status && status.provider_selected && status.provider_selected.provider) || '—') }),
            jsx(KeyValue, { label: t('configPath'), value: (config && config.config_path) || '—' }),
            jsx(KeyValue, { label: t('ingestNs'), value: (config && config.ingest_namespace) || '—' }),
            jsx(KeyValue, { label: t('namespaces'), value: nsStep ? (nsStep.detail || '—') : '—' })
          ] })
        ] })
      })
    ]
  })
}

function SetupPage({ ctx }) {
  const t = usePluginI18n(PLUGIN_ID)
  const api = createSetupApi(ctx)
  const snapshot = useQuery({
    queryKey: [PLUGIN_ID, 'status'],
    queryFn: () => api.getStatus(),
    staleTime: 10_000
  })

  return jsx('div', {
    className: 'h-full min-w-0 w-full',
    style: {
      maxWidth: '100%',
      minWidth: 0,
      overflowX: 'hidden',
      overflowY: 'auto'
    },
    children: jsx('main', {
      className: 'mx-auto flex w-full flex-col gap-4 p-4',
      style: { boxSizing: 'border-box', maxWidth: '48rem' },
      children: jsxs(Fragment, {
        children: [
          jsxs('header', {
            className: 'flex items-center gap-3',
            children: [
              jsx('h2', { className: 'text-xl font-semibold', children: t('title') }),
              jsx(Button, { onClick: () => snapshot.refetch(), children: t('refresh') })
            ]
          }),
          jsx('p', { className: 'mt-1 text-sm', style: MUTED, children: t('subtitle') }),
          snapshot.isLoading ? jsx(EmptyState, { title: t('loading') }) : null,
          snapshot.error ? jsx(EmptyState, { title: t('failed') }) : null,
          snapshot.data ? jsx(StatusCard, { status: snapshot.data, t }) : null,
          snapshot.data ? jsx(Card, {
            className: 'w-full',
            children: jsx(CardContent, { className: 'p-4', children: jsx('p', { className: 'text-sm', style: MUTED, children: t('configuredHere') }) })
          }) : null
        ]
      })
    })
  })
}

function register(ctx) {
  ctx.i18n.register(bundles)
  const navLabel = ctx.i18n.t('title')
  ctx.register({
    id: 'page',
    area: ROUTES_AREA,
    data: { path: PLUGIN_ROUTE },
    render: () => jsx(SetupPage, { ctx })
  })
  ctx.register({
    id: 'nav',
    area: SIDEBAR_NAV_AREA,
    data: { path: PLUGIN_ROUTE, label: navLabel, codicon: 'database' }
  })
  ctx.register({
    id: 'open',
    area: PALETTE_AREA,
    data: {
      id: `${PLUGIN_ID}.open`,
      label: navLabel,
      keywords: ['cortexiq', 'memory', 'setup'],
      run: () => host.navigate(PLUGIN_ROUTE)
    }
  })
}

export default {
  id: PLUGIN_ID,
  name: PLUGIN_NAME,
  version: VERSION,
  register
}
