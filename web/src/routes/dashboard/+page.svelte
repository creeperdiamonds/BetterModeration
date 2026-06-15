<script lang="ts">
  import { onMount } from 'svelte';

  const API_BASE = import.meta.env.VITE_API_URL ?? 'http://localhost:8080';

  type User = { discord_id: string; username: string; issued_at: string };
  type Punishment = {
    id: string;
    profile_id: string;
    type: string;
    reason: string;
    issued_at: string;
    expires_at: string | null;
    revoked_at: string | null;
  };
  type Appeal = { id: string; punishment_id: string; status: string; submitted_at: string };
  type Report = { id: string; category: string; status: string; submitted_at: string };

  let user: User | null = null;
  let loading = true;
  let activePunishments: Punishment[] = [];
  let openAppeals: Appeal[] = [];
  let openReports: Report[] = [];
  let orgId = '';
  let dataError = '';

  function formatDate(iso: string | null): string {
    if (!iso) return '—';
    return new Date(iso).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
  }

  async function fetchDashboardData() {
    if (!orgId.trim()) return;
    dataError = '';
    try {
      const [punishRes, appealsRes, reportsRes] = await Promise.all([
        fetch(`${API_BASE}/v1/profiles/${orgId}/punishments`, { credentials: 'include' }),
        fetch(`${API_BASE}/v1/appeals?org_id=${orgId}&status=PENDING`, { credentials: 'include' }),
        fetch(`${API_BASE}/v1/reports?org_id=${orgId}&status=OPEN`, { credentials: 'include' })
      ]);

      if (punishRes.ok) {
        const all: Punishment[] = await punishRes.json();
        activePunishments = all.filter(
          (p) => !p.revoked_at && (!p.expires_at || new Date(p.expires_at) > new Date())
        ).slice(0, 10);
      }
      if (appealsRes.ok) openAppeals = (await appealsRes.json()).slice(0, 10);
      if (reportsRes.ok) openReports = (await reportsRes.json()).slice(0, 10);
    } catch {
      dataError = 'Failed to load dashboard data.';
    }
  }

  onMount(async () => {
    try {
      const res = await fetch(`${API_BASE}/auth/me`, { credentials: 'include' });
      if (res.ok) {
        user = await res.json();
      }
    } catch {
      // not logged in — user stays null
    } finally {
      loading = false;
    }
  });
</script>

<svelte:head>
  <title>Dashboard — BetterModeration</title>
</svelte:head>

<div style="max-width: 1000px; margin: 0 auto; padding: 3rem 2rem;">
  <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 2rem;">
    <h1 style="font-size: 2rem; font-weight: 800; color: #1a1a2e; margin: 0;">Dashboard</h1>
    {#if user}
      <div style="display: flex; align-items: center; gap: 1rem;">
        <span style="color: #555; font-size: 0.9rem;">Signed in as <strong>{user.username}</strong></span>
        <form method="POST" action="{API_BASE}/auth/logout">
          <button type="submit" style="padding: 0.4rem 1rem; background: #eee; border: 1px solid #ccc; border-radius: 6px; font-size: 0.85rem; cursor: pointer;">
            Log out
          </button>
        </form>
      </div>
    {/if}
  </div>

  {#if loading}
    <p style="color: #888;">Loading…</p>
  {:else if !user}
    <div style="
      background: #fff3cd;
      border: 1px solid #ffc107;
      border-radius: 6px;
      padding: 1rem 1.25rem;
      margin-bottom: 2rem;
      color: #856404;
      font-size: 0.95rem;
    ">
      You must be logged in to view your dashboard.
    </div>
    <a
      href="{API_BASE}/auth/discord"
      style="
        display: inline-block;
        padding: 0.75rem 1.75rem;
        background: #5865F2;
        color: white;
        border-radius: 6px;
        text-decoration: none;
        font-weight: 700;
        font-size: 1rem;
      "
    >
      Login with Discord
    </a>
  {:else}
    <div style="margin-bottom: 1.5rem; display: flex; align-items: center; gap: 0.75rem;">
      <input
        type="text"
        bind:value={orgId}
        placeholder="Org ID to view"
        style="padding: 0.55rem 0.9rem; border: 1px solid #ccc; border-radius: 6px; font-size: 0.95rem; width: 280px;"
      />
      <button
        on:click={fetchDashboardData}
        style="padding: 0.55rem 1.25rem; background: #e94560; color: white; border: none; border-radius: 6px; font-size: 0.95rem; font-weight: 600; cursor: pointer;"
      >
        Load
      </button>
    </div>

    {#if dataError}
      <p style="color: #e94560;">{dataError}</p>
    {/if}

    <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 1.5rem;">
      <!-- Active Punishments -->
      <div style="background: #f9f9f9; border: 1px solid #e0e0e0; border-radius: 8px; padding: 1.5rem;">
        <h2 style="font-size: 1.05rem; font-weight: 700; color: #1a1a2e; margin: 0 0 1rem 0;">
          Active Punishments {#if activePunishments.length > 0}<span style="color: #e94560;">({activePunishments.length})</span>{/if}
        </h2>
        {#if activePunishments.length === 0}
          <p style="font-size: 0.85rem; color: #999; margin: 0;">No active punishments.</p>
        {:else}
          {#each activePunishments as p}
            <div style="padding: 0.5rem 0; border-bottom: 1px solid #eee; font-size: 0.85rem;">
              <span style="font-weight: 600; color: #e94560;">{p.type}</span>
              — {p.reason.slice(0, 60)}{p.reason.length > 60 ? '…' : ''}
              <span style="color: #aaa; display: block; font-size: 0.78rem;">Issued {formatDate(p.issued_at)}</span>
            </div>
          {/each}
        {/if}
      </div>

      <!-- Open Appeals -->
      <div style="background: #f9f9f9; border: 1px solid #e0e0e0; border-radius: 8px; padding: 1.5rem;">
        <h2 style="font-size: 1.05rem; font-weight: 700; color: #1a1a2e; margin: 0 0 1rem 0;">
          Open Appeals {#if openAppeals.length > 0}<span style="color: #f0a500;">({openAppeals.length})</span>{/if}
        </h2>
        {#if openAppeals.length === 0}
          <p style="font-size: 0.85rem; color: #999; margin: 0;">No pending appeals.</p>
        {:else}
          {#each openAppeals as a}
            <div style="padding: 0.5rem 0; border-bottom: 1px solid #eee; font-size: 0.85rem;">
              <span style="font-weight: 600;">{a.id.slice(0, 8)}…</span>
              <span style="color: #aaa; margin-left: 0.5rem;">Punishment: {a.punishment_id.slice(0, 8)}…</span>
              <span style="color: #aaa; display: block; font-size: 0.78rem;">Submitted {formatDate(a.submitted_at)}</span>
            </div>
          {/each}
        {/if}
      </div>

      <!-- Open Reports -->
      <div style="background: #f9f9f9; border: 1px solid #e0e0e0; border-radius: 8px; padding: 1.5rem;">
        <h2 style="font-size: 1.05rem; font-weight: 700; color: #1a1a2e; margin: 0 0 1rem 0;">
          Open Reports {#if openReports.length > 0}<span style="color: #f0a500;">({openReports.length})</span>{/if}
        </h2>
        {#if openReports.length === 0}
          <p style="font-size: 0.85rem; color: #999; margin: 0;">No open reports.</p>
        {:else}
          {#each openReports as rep}
            <div style="padding: 0.5rem 0; border-bottom: 1px solid #eee; font-size: 0.85rem;">
              <span style="font-weight: 600;">{rep.category}</span>
              <span style="color: #aaa; margin-left: 0.5rem; font-size: 0.78rem;">Submitted {formatDate(rep.submitted_at)}</span>
            </div>
          {/each}
        {/if}
      </div>

      <!-- Quick links -->
      <div style="background: #f9f9f9; border: 1px solid #e0e0e0; border-radius: 8px; padding: 1.5rem;">
        <h2 style="font-size: 1.05rem; font-weight: 700; color: #1a1a2e; margin: 0 0 1rem 0;">Quick Links</h2>
        <ul style="list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 0.6rem;">
          <li><a href="/lookup" style="color: #e94560; text-decoration: none; font-size: 0.9rem;">Player Lookup</a></li>
          <li><a href="/appeal" style="color: #e94560; text-decoration: none; font-size: 0.9rem;">Submit Appeal</a></li>
        </ul>
      </div>
    </div>
  {/if}
</div>
