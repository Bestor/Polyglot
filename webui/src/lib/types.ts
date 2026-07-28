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
