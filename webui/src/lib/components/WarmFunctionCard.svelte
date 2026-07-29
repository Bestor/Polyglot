<script lang="ts">
	import { onDestroy } from 'svelte';
	import type { FunctionDescription, Job } from '$lib/types';

	let { datasource, fn }: { datasource: string; fn: FunctionDescription } = $props();

	// Seeded once from fn's initial args - fine here since this component
	// instance is keyed per function (see the parent's {#each fn.id}),
	// so it never needs to re-seed mid-lifetime.
	let values = $state<Record<string, string>>(
		Object.fromEntries(fn.args.map((a) => [a.name, a.type === 'boolean' ? 'false' : '']))
	);
	let submitting = $state(false);
	let job = $state<Job | null>(null);
	let error = $state<string | null>(null);
	let pollHandle: ReturnType<typeof setInterval> | undefined;

	function buildArgs(): Record<string, unknown> {
		const args: Record<string, unknown> = {};
		for (const arg of fn.args) {
			const raw = values[arg.name];
			if (arg.type === 'boolean') {
				args[arg.name] = raw === 'true';
				continue;
			}
			if (raw === '' || raw === undefined) {
				if (arg.required) throw new Error(`"${arg.name}" is required`);
				continue;
			}
			args[arg.name] =
				arg.type === 'integer'
					? Number.parseInt(raw, 10)
					: arg.type === 'number'
						? Number(raw)
						: raw;
		}
		return args;
	}

	function pollJob(jobId: string) {
		clearInterval(pollHandle);
		pollHandle = setInterval(async () => {
			try {
				const params = new URLSearchParams({ id: jobId, datasource });
				const res = await fetch(`/api/warm?${params}`);
				if (!res.ok) throw new Error(await res.text());
				const updated: Job = await res.json();
				job = updated;
				if (updated.status !== 'running') clearInterval(pollHandle);
			} catch (err) {
				error = err instanceof Error ? err.message : String(err);
				clearInterval(pollHandle);
			}
		}, 1500);
	}

	async function submit(e: Event) {
		e.preventDefault();
		if (submitting || job?.status === 'running') return;
		error = null;

		let args: Record<string, unknown>;
		try {
			args = buildArgs();
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
			return;
		}

		submitting = true;
		job = null;
		try {
			const res = await fetch('/api/warm', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ datasource, function: fn.name, args })
			});
			if (!res.ok) throw new Error(await res.text());
			const created: Job = await res.json();
			job = created;
			pollJob(created.id);
		} catch (err) {
			error = err instanceof Error ? err.message : String(err);
		} finally {
			submitting = false;
		}
	}

	onDestroy(() => clearInterval(pollHandle));
</script>

<div class="function-card">
	<div class="function-name">{fn.name}</div>
	{#if fn.description}
		<p class="function-description">{fn.description}</p>
	{/if}
	{#if fn.query_guidance}
		<p class="function-guidance"><strong>Guidance:</strong> {fn.query_guidance}</p>
	{/if}

	<form onsubmit={submit}>
		{#each fn.args as arg (arg.name)}
			{#if arg.type === 'boolean'}
				<label class="arg-row arg-bool">
					<input
						type="checkbox"
						checked={values[arg.name] === 'true'}
						onchange={(e) =>
							(values[arg.name] = (e.currentTarget as HTMLInputElement).checked
								? 'true'
								: 'false')}
					/>
					<span>{arg.name}</span>
				</label>
			{:else}
				<label class="arg-row">
					<span class="arg-label">{arg.name}{arg.required ? ' *' : ''}</span>
					<input
						type={arg.type === 'integer' || arg.type === 'number' ? 'number' : 'text'}
						bind:value={values[arg.name]}
						placeholder={arg.description}
						title={arg.description}
					/>
				</label>
			{/if}
		{/each}
		<button type="submit" disabled={submitting || job?.status === 'running'}>
			{submitting ? 'Starting…' : job?.status === 'running' ? 'Running…' : 'Run'}
		</button>
	</form>

	{#if job}
		<div class="job-status job-status-{job.status}">
			{#if job.status === 'running'}
				Running (job {job.id})…
			{:else if job.status === 'succeeded'}
				Done{job.summary ? ` — ${job.summary}` : ''}
			{:else}
				Failed{job.error ? ` — ${job.error}` : ''}
			{/if}
		</div>
	{/if}
	{#if error}
		<div class="function-error">{error}</div>
	{/if}
</div>

<style>
	.function-card {
		background: var(--muted-bg, #f1e6cd);
		border: 1px solid var(--border, #d8c8a0);
		border-radius: 0.6rem;
		padding: 0.6rem 0.75rem;
	}
	.function-name {
		font-family: monospace;
		font-weight: 600;
		color: #3f3826;
	}
	.function-description {
		font-size: 0.82rem;
		color: var(--muted-fg, #8a7a54);
		margin: 0.3rem 0;
	}
	.function-guidance {
		font-size: 0.78rem;
		color: var(--muted-fg, #8a7a54);
		margin: 0.3rem 0;
	}
	form {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		margin-top: 0.5rem;
	}
	.arg-row {
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
	}
	.arg-bool {
		flex-direction: row;
		align-items: center;
		gap: 0.4rem;
	}
	.arg-label {
		font-size: 0.72rem;
		font-family: monospace;
		color: var(--muted-fg, #8a7a54);
	}
	.arg-row input[type='text'],
	.arg-row input[type='number'] {
		font-size: 0.8rem;
		padding: 0.3rem 0.5rem;
		border: 1px solid var(--border, #d8c8a0);
		border-radius: 0.4rem;
		background: var(--panel-bg, #fffdf8);
	}
	.arg-row input[type='text']:focus,
	.arg-row input[type='number']:focus {
		outline: none;
		border-color: var(--accent, #5f9a6f);
		box-shadow: 0 0 0 2px color-mix(in srgb, var(--accent, #5f9a6f) 18%, transparent);
	}
	form button {
		align-self: flex-start;
		margin-top: 0.2rem;
		padding: 0.35rem 0.9rem;
		border: none;
		border-radius: 0.4rem;
		background: var(--accent, #5f9a6f);
		color: #fff;
		font-weight: 600;
		font-size: 0.8rem;
		cursor: pointer;
		transition: background 0.15s ease;
	}
	form button:hover:not(:disabled) {
		background: var(--accent-hover, #4f8a5f);
	}
	form button:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.job-status {
		margin-top: 0.5rem;
		font-size: 0.78rem;
		border-radius: 0.4rem;
		padding: 0.3rem 0.5rem;
	}
	.job-status-running {
		background: var(--panel-bg, #fffdf8);
		color: var(--muted-fg, #8a7a54);
		font-style: italic;
	}
	.job-status-succeeded {
		background: var(--success-bg, #dbeedb);
		color: var(--success-fg, #2f6b3f);
	}
	.job-status-failed {
		background: var(--error-bg, #f7e3d6);
		color: var(--error-fg, #a1502b);
	}
	.function-error {
		margin-top: 0.4rem;
		font-size: 0.78rem;
		color: var(--error-fg, #a1502b);
	}
</style>
