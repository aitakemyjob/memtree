// Wire types, mirrored 1:1 from the Go backend (internal/store, internal/core,
// internal/config, internal/hooks, internal/api). The Go side is the source
// of truth: when a struct changes there, change it here.
//
// Timestamps are RFC 3339 strings. Lists are never null on the wire, but the
// UI still tolerates null with `?? []` at the API boundary (see api.ts).

/** Memory tier. */
export type Tier = 'episodic' | 'semantic';
/** Provenance: how trustworthy a memory is. */
export type Provenance = 'HUMAN_CONFIRMED' | 'SOURCE_GROUNDED' | 'MODEL_INFERRED';
/** Who placed a lattice link. */
export type LinkSource = 'agent' | 'human' | 'auto';
/** Memory kinds the backend names; any other string is allowed. */
export type KnownKind = 'note' | 'decision' | 'lesson' | 'session' | 'capture';

/** store.MemoryRecord */
export interface MemoryRecord {
  id: string;
  createdAt: string;
  updatedAt: string;
  /** Repo path; "" = global. */
  project: string;
  /** claude-code, codex, cursor, vscode, mcp, ui... */
  harness: string;
  sessionId?: string;
  tier: Tier;
  kind: string;
  summary: string;
  keywords: string[];
  decisions: string[];
  openLoops: string[];
  /** 1 routine, 1.5 decision, 2.5 lesson. */
  importance: number;
  provenance: string;
  pinned: boolean;
  accessLog: string[];
  supersededBy?: string;
  archivedAt?: string;
  latticePaths: string[];
}

/** store.ScoreDecomposition: final = bm25Normalized x importanceOffset x activation x projectBoost. */
export interface ScoreDecomposition {
  bm25Normalized: number;
  importanceOffset: number;
  activation: number;
  projectBoost: number;
  final: number;
}

/** core.Recalled */
export interface Recalled {
  record: MemoryRecord;
  score: ScoreDecomposition;
  source: 'text' | 'lattice' | 'both';
  via?: string;
}

/** core.RecallResult */
export interface RecallResult {
  memories: Recalled[];
  categories: string[];
}

/** store.CategoryNode */
export interface CategoryNode {
  /** Full slash path. */
  id: string;
  /** "" for an axis root. */
  parentId: string;
  label: string;
  createdAt: string;
  accessCount: number;
  lastAccessed: string;
  linkCount: number;
  /** Current time-decay multiplier, 0.15..1. */
  activation: number;
}

/** store.CategoryLink */
export interface CategoryLink {
  itemType: string;
  itemId: string;
  categoryId: string;
  weight: number;
  summary: string;
  source: LinkSource;
  createdAt: string;
  lastAccessed: string;
}

/** store.ActivationHit */
export interface ActivationHit {
  itemType: string;
  itemId: string;
  score: number;
  viaCategory: string;
  summary: string;
}

/** store.UntaggedItem */
export interface UntaggedItem {
  itemType: string;
  itemId: string;
  text: string;
  at: string;
}

/** store.CaptureSession */
export interface CaptureSession {
  id: string;
  harness: string;
  project: string;
  startedAt: string;
  lastAt: string;
  closedAt?: string;
  closeReason: string;
  /** The raw capture memory. */
  recordId: string;
  summarizedAt?: string;
  turns: number;
}

/** store.CaptureTurn */
export interface CaptureTurn {
  seq: number;
  role: 'user' | 'assistant' | string;
  text: string;
  at: string;
}

/** store.TierConfig */
export interface TierConfig {
  ageHoursToSemantic: number;
  ageHoursToCompact: number;
  activationCeilingToCompress: number;
  minClusterSize: number;
}

/** store.Cluster */
export interface Cluster {
  records: MemoryRecord[];
  project: string;
  sharedKeywords: string[];
}

/** store.Stats */
export interface Stats {
  total: number;
  episodic: number;
  semantic: number;
  pinned: number;
  superseded: number;
  archived: number;
  untagged: number;
  nodes: number;
  links: number;
  capturesPending: number;
}

/** store.ProjectCount ("" = global). */
export interface ProjectCount {
  project: string;
  count: number;
}

/** config.Settings */
export interface Settings {
  theme: string;
  fontSize: number;
  mcpPort: number;
  dbPath: string;
  agentWrites: boolean;
  captureHooks: boolean;
  surfaceThreshold: number;
  maxSurface: number;
  linkHalfLifeDays: number;
  maintainHours: number;
  tiers: TierConfig;
}

/** core.MCPStatus */
export interface MCPStatus {
  running: boolean;
  url: string;
  error?: string;
}

/** core.Activity */
export interface Activity {
  id: number;
  time: string;
  /** mcp | hook | ui */
  source: string;
  harness: string;
  tool: string;
  summary: string;
  project: string;
  error?: string;
  durationMs: number;
}

/** api.AppState */
export interface AppState {
  version: string;
  platform: string;
  settings: Settings;
  mcp: MCPStatus;
  stats: Stats;
  projects: ProjectCount[];
  axisRoots: string[];
  configPath: string;
  canPickDir: boolean;
}

/** hooks.Change */
export interface HookChange {
  file: string;
  /** add | update | remove | run | note */
  action: string;
  detail: string;
}

/** hooks.Result */
export interface HookResult {
  harness: string;
  dryRun: boolean;
  changes: HookChange[];
  notes: string[];
}

/** hooks.HarnessStatus */
export interface HarnessStatus {
  harness: string;
  name: string;
  detected: boolean;
  hooks: boolean;
  mcp: boolean;
  files: string[];
  limits?: string;
  events: string[];
}

/** hooks.Status */
export interface HooksStatus {
  exe: string;
  harnesses: HarnessStatus[];
}

/** memory.explain */
export interface Explained {
  record: MemoryRecord;
  activation: number;
  ageHours: number;
  compressible: boolean;
  links: CategoryLink[];
}

/** lattice.fire */
export interface FireResult {
  seeds: string[];
  hits: ActivationHit[];
}

/** Event envelope published on the bus (events.Event). */
export interface BusEvent {
  type: 'config' | 'activity' | 'memory' | 'lattice' | 'capture' | 'mcp' | 'focus' | string;
  time?: string;
  data?: unknown;
}

/** Arguments shared by memory.list and memory.recall. */
export interface ListArgs {
  project?: string;
  allProjects?: boolean;
  tier?: string;
  kind?: string;
  /** Hide one kind, e.g. 'capture'. */
  excludeKind?: string;
  limit?: number;
  includeSuperseded?: boolean;
  includeArchived?: boolean;
  pinnedOnly?: boolean;
  before?: string;
  after?: string;
}

/** Arguments of memory.recall. */
export interface RecallArgs extends ListArgs {
  query: string;
  /** Strengthen what is returned; the UI leaves this off so looking never changes data. */
  reinforce?: boolean;
}

/** Arguments of memory.update; omitted fields are left alone. */
export interface UpdateArgs {
  id: string;
  summary?: string;
  keywords?: string[];
  decisions?: string[];
  openLoops?: string[];
  importance?: number;
  kind?: string;
  project?: string;
  provenance?: string;
  latticePaths?: string[];
  /** true replaces links with latticePaths, even when empty. */
  setPaths?: boolean;
}

/** The fields a human edits; used by the memory form for both create and update. */
export interface MemoryDraft {
  summary: string;
  keywords: string[];
  decisions: string[];
  openLoops: string[];
  importance: number;
  kind: string;
  project: string;
  provenance: string;
  latticePaths: string[];
  tier: Tier;
}

/** store.NearDuplicate */
export interface NearDuplicate {
  a: string;
  b: string;
  reason: string;
  linksA: number;
  linksB: number;
}

/** store.SingleUse */
export interface SingleUse {
  category: string;
  memoryId: string;
  summary: string;
}

/** store.WeaklyFiled */
export interface WeaklyFiled {
  memoryId: string;
  summary: string;
  paths: string[];
}

/** store.LintReport: each list is capped server-side; total counts findings before the cap. */
export interface LintReport {
  nearDuplicates: NearDuplicate[];
  outsideAxes: string[];
  singleUse: SingleUse[];
  weaklyFiled: WeaklyFiled[];
  total: number;
}

/** store.PathWarning: advice about a path about to be filed under. */
export interface PathWarning {
  path: string;
  message: string;
  /** An existing category to use instead. */
  suggest?: string;
}
