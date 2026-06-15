<script lang="ts">
  const API_BASE = import.meta.env.VITE_API_URL ?? 'http://localhost:8080';

  type Punishment = {
    id: string;
    type: string;
    reason: string;
    issued_at: string;
    expires_at: string | null;
    revoked_at: string | null;
    public: boolean;
  };

  type Profile = {
    id: string;
    discord_id: string | null;
    minecraft_uuid: string | null;
    username: string | null;
  };

  let query = '';
  let profile: Profile | null = null;
  let punishments: Punishment[] = [];
  let searched = false;
  let loading = false;
  let error = '';

  function formatDate(iso: string | null): string {
    if (!iso) return '—';
    return new Date(iso).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
  }

  function punishmentStatus(p: Punishment): string {
    if (p.revoked_at) return 'Revoked';
    if (p.expires_at && new Date(p.expires_at) < new Date()) return 'Expired';
    return 'Active';
  }

  function statusColor(status: string): string {
    if (status === 'Active') return '#e94560';
    if (status === 'Revoked') return '#28a745';
    return '#888';
  }

  async function handleSearch() {
    if (!query.trim()) return;
    loading = true;
    error = '';
    profile = null;
    punishments = [];
    searched = false;

    try {
      // Try to resolve the profile by ID (works for Discord ID, Minecraft UUID, or profile ID)
      const profileRes = await fetch(`${API_BASE}/v1/profiles/${encodeURIComponent(query.trim())}`);
      if (!profileRes.ok) {
        error = 'No profile found for that query.';
        searched = true;
        return;
      }
      profile = await profileRes.json();

      const punishRes = await fetch(`${API_BASE}/v1/profiles/${profile!.id}/punishments`);
      if (punishRes.ok) {
        const all: Punishment[] = await punishRes.json();
        // Only show public punishments on this page
        punishments = all.filter((p) => p.public);
      }
      searched = true;
    } catch (e) {
      error = 'Failed to reach the API. Please try again later.';
      searched = true;
    } finally {
      loading = false;
    }
  }
</script>

<svelte:head>
  <title>Player Lookup — BetterModeration</title>
</svelte:head>

<div style="max-width: 900px; margin: 0 auto; padding: 3rem 2rem;">
  <h1 style="font-size: 2rem; font-weight: 800; color: #1a1a2e; margin-bottom: 0.5rem;">Player Lookup</h1>
  <p style="color: #555; margin-bottom: 2rem; font-size: 0.95rem;">
    Search for a player's moderation history by Discord ID, Minecraft username, or UUID.
  </p>

  <form on:submit|preventDefault={handleSearch} style="display: flex; gap: 0.75rem; margin-bottom: 2rem;">
    <input
      type="text"
      bind:value={query}
      placeholder="Discord ID, Minecraft username or UUID"
      style="
        flex: 1;
        padding: 0.65rem 1rem;
        border: 1px solid #ccc;
        border-radius: 6px;
        font-size: 1rem;
        outline: none;
      "
    />
    <button
      type="submit"
      disabled={loading}
      style="
        padding: 0.65rem 1.5rem;
        background: #e94560;
        color: white;
        border: none;
        border-radius: 6px;
        font-size: 1rem;
        font-weight: 600;
        cursor: pointer;
      "
    >
      {loading ? 'Searching…' : 'Search'}
    </button>
  </form>

  {#if error}
    <p style="color: #e94560; font-style: italic;">{error}</p>
  {/if}

  {#if searched && !error}
    {#if profile}
      <div style="background: #f0f4ff; border: 1px solid #c7d2fe; border-radius: 8px; padding: 1rem 1.25rem; margin-bottom: 1.5rem;">
        <p style="margin: 0; font-size: 0.9rem; color: #333;">
          <strong>Profile:</strong> {profile.username ?? profile.id}
          {#if profile.discord_id}<span style="color: #777;"> — Discord: {profile.discord_id}</span>{/if}
          {#if profile.minecraft_uuid}<span style="color: #777;"> — Minecraft: {profile.minecraft_uuid}</span>{/if}
        </p>
      </div>
    {/if}

    {#if punishments.length === 0}
      <p style="color: #888; font-style: italic;">No public punishment records found for "{query}".</p>
    {:else}
      <div style="overflow-x: auto;">
        <table style="width: 100%; border-collapse: collapse; font-size: 0.9rem;">
          <thead>
            <tr style="background: #1a1a2e; color: white; text-align: left;">
              <th style="padding: 0.75rem 1rem;">Type</th>
              <th style="padding: 0.75rem 1rem;">Reason</th>
              <th style="padding: 0.75rem 1rem;">Issued</th>
              <th style="padding: 0.75rem 1rem;">Expires</th>
              <th style="padding: 0.75rem 1rem;">Status</th>
            </tr>
          </thead>
          <tbody>
            {#each punishments as p, i}
              {@const status = punishmentStatus(p)}
              <tr style="background: {i % 2 === 0 ? '#f9f9f9' : 'white'}; border-bottom: 1px solid #eee;">
                <td style="padding: 0.65rem 1rem; font-weight: 600;">{p.type}</td>
                <td style="padding: 0.65rem 1rem;">{p.reason}</td>
                <td style="padding: 0.65rem 1rem;">{formatDate(p.issued_at)}</td>
                <td style="padding: 0.65rem 1rem;">{formatDate(p.expires_at)}</td>
                <td style="padding: 0.65rem 1rem; font-weight: 600; color: {statusColor(status)};">{status}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  {/if}
</div>
