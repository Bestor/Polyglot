// Mirrors internal/polyglot/queries.go's QuerySummary/QueryDetail/SpanDetail
// exactly (field names match their `json:` tags).

export interface QuerySummary {
	id: string;
	sql: string;
	question: string | null;
	status: 'success' | 'error';
	duration_ms: number;
	timestamp: string;
}

export interface SpanDetail {
	service: string;
	operation_name: string;
	duration_ms: number;
	tags: Record<string, string>;
}

export interface QueryDetail extends QuerySummary {
	response: string;
	spans: SpanDetail[];
}

export interface ListQueriesResponse {
	queries: QuerySummary[];
}

// Mirrors internal/polyglot/datasources.go's DatasourcesResponse/
// ActiveDatasourceDescription exactly.

export interface ActiveDatasourceDescription {
	name: string;
	type: string;
}

export interface DatasourcesResponse {
	active: ActiveDatasourceDescription[];
}

// Mirrors internal/polyglot/metadata.go's MetadataResponse and friends
// exactly (field names match their `json:` tags).

export interface ColumnDescription {
	id: string;
	name: string;
	type: string;
	description: string;
	// Introspected (not curated), refreshed every reconcile - empty when no
	// relation is mechanically known. See internal/polyglot/catalog.go's
	// reconcileColumns.
	references_table?: string;
	references_column?: string;
}

export interface ExampleQuery {
	question: string;
	sql: string;
}

export interface GlossaryEntry {
	term: string;
	definition: string;
}

export interface TableDescription {
	id: string;
	name: string;
	description: string;
	datasource: string;
	query_guidance: string;
	// Curated - only ever change via POST /tables/annotate.
	good_for: string;
	bad_for: string;
	known_gaps: string;
	example_queries: ExampleQuery[];
	// Introspected (not curated) - computed fresh every reconcile by
	// internal/polyglot/catalog.go's reconcileTableStats. last_updated is
	// best-effort (empty if the table has no "updated" column).
	row_count: number;
	sample_rows: Record<string, unknown>[];
	last_updated?: string;
	columns: ColumnDescription[];
}

export interface DatasourceGuidance {
	name: string;
	description: string;
	query_guidance: string;
	// Curated, connection-level - see internal/polyglot/metadata.go.
	glossary: GlossaryEntry[];
	example_queries: ExampleQuery[];
}

export interface FunctionArgDescription {
	name: string;
	type: string;
	description: string;
	required: boolean;
}

export interface FunctionDescription {
	id: string;
	name: string;
	description: string;
	datasource: string;
	query_guidance: string;
	args: FunctionArgDescription[];
}

export interface MetadataResponse {
	datasources: DatasourceGuidance[];
	tables: TableDescription[];
	functions: FunctionDescription[];
}

// Mirrors internal/polyglot/query.go's QueryResponse exactly - the shape
// GET /query (proxied via routes/api/explorer/.../+server.ts and
// routes/api/query/[datasource]/+server.ts) returns.
export interface TableRows {
	rows: Record<string, unknown>[];
	row_count: number;
	truncated: boolean;
}

// Mirrors internal/jobstore.Job exactly - the shape POST /warm (202) and
// GET /jobs return, proxied via routes/api/warm/+server.ts.
export type JobStatus = 'running' | 'succeeded' | 'failed';

export interface Job {
	id: string;
	datasource: string;
	function: string;
	status: JobStatus;
	summary?: string;
	data?: Record<string, unknown>;
	error?: string;
	created_at: string;
	updated_at: string;
}
