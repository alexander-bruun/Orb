<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { ingestStatus, type LastScan } from '$lib/stores/ingestStatus';

  let open = false;
  let triggerEl: HTMLButtonElement;

  export let closeOther: () => void = () => {};

  function toggle(e: MouseEvent) {
    e.stopPropagation();
    if (!open) closeOther();
    open = !open;
  }

  export function close() {
    open = false;
  }

  function formatTime(iso: string): string {
    const d = new Date(iso);
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }

  function formatDuration(a: string, b: string): string {
    const ms = new Date(b).getTime() - new Date(a).getTime();
    const s = Math.round(ms / 1000);
    if (s < 60) return `${s}s`;
    return `${Math.floor(s / 60)}m ${s % 60}s`;
  }

  function shortenPath(p: string | undefined): string {
    if (!p) return '';
    const parts = p.split(/[\\/]/);
    if (parts.length <= 2) return p;
    return '…/' + parts.slice(-2).join('/');
  }

  $: phase = $ingestStatus.phase;
  $: running = $ingestStatus.running;
  $: done = $ingestStatus.done;
  $: total = $ingestStatus.total;
  $: progress = total > 0 ? Math.min(1, done / total) : 0;
  $: lastScan = $ingestStatus.lastScan as LastScan | undefined;
  $: currentFile = $ingestStatus.currentFile;
  $: startedAt = $ingestStatus.startedAt;

  $: etc = (() => {
    if (!running || !startedAt || done <= 0 || total <= 0 || done >= total) return null;
    const elapsed = Date.now() - startedAt;
    const rate = done / elapsed; // items per ms
    const remaining = total - done;
    const msRemaining = remaining / rate;
    
    if (msRemaining < 1000) return '< 1s';
    const s = Math.round(msRemaining / 1000);
    if (s < 60) return `${s}s remaining`;
    const m = Math.floor(s / 60);
    if (m < 60) return `${m}m ${s % 60}s remaining`;
    const h = Math.floor(m / 60);
    return `${h}h ${m % 60}m remaining`;
  })();

  onMount(() => ingestStatus.init());
  onDestroy(() => ingestStatus.destroy());
</script>

<svelte:window on:click={close} />

<div class="ingest-wrap" role="presentation" tabindex="-1" on:click|stopPropagation on:keydown|stopPropagation>
  <button
    bind:this={triggerEl}
    class="ingest-btn"
    class:running
    class:has-error={phase === 'error'}
    on:click={toggle}
    aria-label="Ingest status"
    title={running ? `Library scan in progress${etc ? ` (${etc})` : ''}` : 'Library ingest status'}
  >
    {#if running}
      <!-- Spinning sync icon -->
      <svg class="spin-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M21 2v6h-6"/>
        <path d="M3 12a9 9 0 0 1 15-6.7L21 8"/>
        <path d="M3 22v-6h6"/>
        <path d="M21 12a9 9 0 0 1-15 6.7L3 16"/>
      </svg>
    {:else}
      <!-- Database icon -->
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <ellipse cx="12" cy="5" rx="9" ry="3"/>
        <path d="M3 5v14c0 1.66 4.03 3 9 3s9-1.34 9-3V5"/>
        <path d="M3 12c0 1.66 4.03 3 9 3s9-1.34 9-3"/>
      </svg>
    {/if}
    {#if phase === 'error'}
      <span class="dot dot--error"></span>
    {:else if running}
      <span class="dot dot--active"></span>
    {:else if phase === 'complete'}
      <span class="dot dot--ok"></span>
    {/if}
  </button>

  {#if open}
      <div class="panel" role="dialog" tabindex="-1" on:click|stopPropagation on:keydown|stopPropagation>
      <div class="panel-header">
        <span class="panel-title">Library Ingest</span>
        <span class="status-badge" class:badge--running={running} class:badge--error={phase === 'error'} class:badge--ok={phase === 'complete' && !running} class:badge--idle={phase === 'idle'}>
          {#if running}Scanning{:else if phase === 'error'}Error{:else if phase === 'complete'}Done{:else}Idle{/if}
        </span>
      </div>

      {#if running}
        <div class="section">
          <div class="progress-row">
            <div class="progress-track">
              <div class="progress-fill" style="width: {progress * 100}%"></div>
            </div>
            <span class="progress-label">{Math.round(progress * 100)}%</span>
          </div>

          {#if etc}
            <div class="etc-label">{etc}</div>
          {/if}

          {#if currentFile}
            <div class="current-file">
              <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/></svg>
              {shortenPath(currentFile)}
            </div>
          {/if}

          <div class="counters">
            <span class="counter"><span class="counter-val">{done}</span>/{total} ingested</span>
            {#if $ingestStatus.skipped > 0}<span class="counter"><span class="counter-val">{$ingestStatus.skipped}</span> skipped</span>{/if}
            {#if $ingestStatus.errors > 0}<span class="counter counter--err"><span class="counter-val">{$ingestStatus.errors}</span> errors</span>{/if}
          </div>
        </div>
      {:else if lastScan}
        <div class="section">
          <div class="scan-meta">
            <span class="scan-time">{formatTime(lastScan.started_at)}</span>
            <span class="scan-dur">{formatDuration(lastScan.started_at, lastScan.finished_at)}</span>
          </div>
          <div class="counters">
            {#if (lastScan.ingested ?? lastScan.enqueued) > 0}
              <span class="counter">
                <span class="counter-val">{lastScan.ingested ?? lastScan.enqueued}</span>
                {lastScan.enqueued > 0 && !lastScan.ingested ? 'queued' : 'ingested'}
              </span>
            {/if}
            {#if lastScan.skipped > 0}<span class="counter"><span class="counter-val">{lastScan.skipped}</span> skipped</span>{/if}
            {#if lastScan.errors > 0}<span class="counter counter--err"><span class="counter-val">{lastScan.errors}</span> errors</span>{/if}
            {#if !lastScan.ingested && !lastScan.enqueued && !lastScan.skipped && !lastScan.errors}
              <span class="counter"><span class="counter-val">0</span> changes</span>
            {/if}
          </div>
        </div>
      {:else}
        <div class="section empty-state">No scans yet</div>
      {/if}

    </div>
  {/if}
</div>

<style>
  .ingest-wrap {
    position: relative;
    flex-shrink: 0;
  }

  .ingest-btn {
    position: relative;
    width: 30px;
    height: 30px;
    border-radius: 7px;
    background: none;
    border: 1px solid transparent;
    color: var(--text-muted);
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    transition: color 0.15s, background 0.15s, border-color 0.15s;
  }

  .ingest-btn:hover {
    color: var(--text);
    background: var(--surface-2);
    border-color: var(--border);
  }

  .ingest-btn.running {
    color: var(--accent);
    border-color: var(--accent-glow);
    background: var(--accent-dim);
  }

  .ingest-btn.has-error {
    color: #f87171;
    border-color: rgba(248, 113, 113, 0.3);
    background: rgba(248, 113, 113, 0.08);
  }

  .spin-icon {
    animation: spin 1.4s linear infinite;
  }

  @keyframes spin {
    from { transform: rotate(0deg); }
    to   { transform: rotate(360deg); }
  }

  .dot {
    position: absolute;
    top: 4px;
    right: 4px;
    width: 6px;
    height: 6px;
    border-radius: 50%;
    border: 1.5px solid var(--bg-elevated, #0a0a0b);
  }

  .dot--active {
    background: var(--accent);
    animation: pulse-dot 1.4s ease-in-out infinite;
  }

  .dot--ok {
    background: #4ade80;
  }

  .dot--error {
    background: #f87171;
  }

  @keyframes pulse-dot {
    0%, 100% { opacity: 1; transform: scale(1); }
    50%       { opacity: 0.6; transform: scale(0.8); }
  }

  /* ── Panel ─────────────────────────────────────────── */
  .panel {
    position: absolute;
    top: calc(100% + 8px);
    right: 0;
    width: 260px;
    background: var(--surface);
    border: 1px solid var(--border-2);
    border-radius: 10px;
    box-shadow: 0 10px 36px rgba(0, 0, 0, 0.45);
    overflow: hidden;
    z-index: 200;
    animation: panel-in 0.12s ease;
  }

  @keyframes panel-in {
    from { opacity: 0; transform: translateY(-5px) scale(0.98); }
    to   { opacity: 1; transform: translateY(0) scale(1); }
  }

  .panel-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 12px 14px 10px;
  }

  .panel-title {
    font-size: 12px;
    font-weight: 600;
    color: var(--text);
    font-family: 'Syne', sans-serif;
  }

  .status-badge {
    font-size: 10px;
    font-family: 'DM Mono', monospace;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    border-radius: 4px;
    padding: 2px 7px;
  }

  .badge--running {
    color: var(--accent);
    background: var(--accent-dim);
    border: 1px solid var(--accent-glow);
  }

  .badge--ok {
    color: #4ade80;
    background: rgba(74, 222, 128, 0.1);
    border: 1px solid rgba(74, 222, 128, 0.25);
  }

  .badge--error {
    color: #f87171;
    background: rgba(248, 113, 113, 0.1);
    border: 1px solid rgba(248, 113, 113, 0.25);
  }

  .badge--idle {
    color: var(--text-muted);
    background: var(--surface-2);
    border: 1px solid var(--border);
  }

  .section {
    padding: 0 14px 12px;
    border-top: 1px solid var(--border);
  }

  .empty-state {
    font-size: 12px;
    color: var(--text-muted);
    padding: 12px 14px;
    text-align: center;
  }

  /* ── Progress ───────────────────────────────────────── */
  .progress-row {
    display: flex;
    align-items: center;
    gap: 8px;
    padding-top: 12px;
  }

  .progress-track {
    flex: 1;
    height: 3px;
    background: var(--surface-2);
    border-radius: 2px;
    overflow: hidden;
  }

  .progress-fill {
    height: 100%;
    background: var(--accent);
    border-radius: 2px;
    transition: width 0.3s ease;
    min-width: 4px;
  }

  .progress-label {
    font-size: 10px;
    font-family: 'DM Mono', monospace;
    color: var(--text-muted);
    flex-shrink: 0;
  }

  .etc-label {
    margin-top: 4px;
    font-size: 10px;
    font-family: 'DM Mono', monospace;
    color: var(--accent);
    text-align: right;
    font-weight: 500;
  }

  .current-file {
    margin-top: 6px;
    font-size: 10px;
    font-family: 'DM Mono', monospace;
    color: var(--text-muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    display: flex;
    align-items: center;
    gap: 5px;
  }

  /* ── Counters ───────────────────────────────────────── */
  .counters {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-top: 10px;
  }

  .counter {
    font-size: 10px;
    font-family: 'DM Mono', monospace;
    color: var(--text-muted);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: 4px;
    padding: 2px 7px;
    white-space: nowrap;
  }

  .counter--err {
    color: #f87171;
    background: rgba(248, 113, 113, 0.08);
    border-color: rgba(248, 113, 113, 0.2);
  }

  .counter-val {
    color: var(--text);
    font-weight: 600;
  }

  /* ── Scan meta (last scan time) ─────────────────────── */
  .scan-meta {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding-top: 12px;
  }

  .scan-time {
    font-size: 11px;
    color: var(--text-2);
    font-family: 'DM Mono', monospace;
  }

  .scan-dur {
    font-size: 10px;
    color: var(--text-muted);
    font-family: 'DM Mono', monospace;
  }

</style>
