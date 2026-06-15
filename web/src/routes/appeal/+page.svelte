<script lang="ts">
  const API_BASE = import.meta.env.VITE_API_URL ?? 'http://localhost:8080';

  let punishmentId = '';
  let profileId = '';
  let appealReason = '';
  let evidence = '';
  let submitted = false;
  let submitting = false;
  let errorMsg = '';

  async function handleSubmit() {
    if (!punishmentId.trim() || !profileId.trim() || !appealReason.trim()) return;
    submitting = true;
    errorMsg = '';

    try {
      const res = await fetch(`${API_BASE}/v1/appeals`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          punishment_id: punishmentId.trim(),
          profile_id: profileId.trim(),
          reason: appealReason.trim(),
          evidence: evidence.trim() || null
        })
      });

      if (res.status === 201) {
        submitted = true;
      } else {
        const body = await res.json().catch(() => ({}));
        errorMsg = body.error ?? `Server returned ${res.status}. Please check your Punishment ID and Profile ID.`;
      }
    } catch {
      errorMsg = 'Failed to reach the API. Please try again later.';
    } finally {
      submitting = false;
    }
  }
</script>

<svelte:head>
  <title>Submit Appeal — BetterModeration</title>
</svelte:head>

<div style="max-width: 640px; margin: 0 auto; padding: 3rem 2rem;">
  <h1 style="font-size: 2rem; font-weight: 800; color: #1a1a2e; margin-bottom: 0.5rem;">Submit an Appeal</h1>
  <p style="color: #555; margin-bottom: 2rem; font-size: 0.95rem;">
    Use this form to appeal a punishment you believe was issued incorrectly or unfairly.
  </p>

  <div style="
    background: #fff3cd;
    border: 1px solid #ffc107;
    border-radius: 6px;
    padding: 0.9rem 1.25rem;
    margin-bottom: 2rem;
    color: #856404;
    font-size: 0.9rem;
    line-height: 1.5;
  ">
    <strong>Note:</strong> Submitting false or misleading appeals may result in additional restrictions
    on your account. Please be honest and provide accurate information.
  </div>

  {#if submitted}
    <div style="
      background: #d4edda;
      border: 1px solid #28a745;
      border-radius: 6px;
      padding: 1rem 1.25rem;
      color: #155724;
      font-size: 0.95rem;
    ">
      Your appeal has been submitted successfully. You will be notified via Discord or Minecraft when a decision is made.
    </div>
  {:else}
    {#if errorMsg}
      <div style="
        background: #f8d7da;
        border: 1px solid #e94560;
        border-radius: 6px;
        padding: 0.75rem 1.25rem;
        color: #721c24;
        font-size: 0.9rem;
        margin-bottom: 1.25rem;
      ">
        {errorMsg}
      </div>
    {/if}

    <form on:submit|preventDefault={handleSubmit} style="display: flex; flex-direction: column; gap: 1.25rem;">
      <div>
        <label for="punishmentId" style="display: block; font-weight: 600; color: #1a1a2e; margin-bottom: 0.4rem; font-size: 0.95rem;">
          Punishment ID <span style="color: #e94560;">*</span>
        </label>
        <input
          id="punishmentId"
          type="text"
          bind:value={punishmentId}
          required
          placeholder="e.g. a3f1c2d4-..."
          style="width: 100%; padding: 0.65rem 0.9rem; border: 1px solid #ccc; border-radius: 6px; font-size: 1rem; box-sizing: border-box;"
        />
      </div>

      <div>
        <label for="profileId" style="display: block; font-weight: 600; color: #1a1a2e; margin-bottom: 0.4rem; font-size: 0.95rem;">
          Your Profile ID <span style="color: #e94560;">*</span>
        </label>
        <input
          id="profileId"
          type="text"
          bind:value={profileId}
          required
          placeholder="Your BetterModeration profile ID"
          style="width: 100%; padding: 0.65rem 0.9rem; border: 1px solid #ccc; border-radius: 6px; font-size: 1rem; box-sizing: border-box;"
        />
        <p style="font-size: 0.8rem; color: #888; margin: 0.3rem 0 0 0;">
          Find your profile ID on your punishment record or by searching the <a href="/lookup" style="color: #e94560;">player lookup</a> page.
        </p>
      </div>

      <div>
        <label for="appealReason" style="display: block; font-weight: 600; color: #1a1a2e; margin-bottom: 0.4rem; font-size: 0.95rem;">
          Reason for appeal <span style="color: #e94560;">*</span>
        </label>
        <textarea
          id="appealReason"
          bind:value={appealReason}
          required
          rows={5}
          placeholder="Explain why you believe this punishment should be reconsidered..."
          style="width: 100%; padding: 0.65rem 0.9rem; border: 1px solid #ccc; border-radius: 6px; font-size: 1rem; box-sizing: border-box; resize: vertical;"
        ></textarea>
      </div>

      <div>
        <label for="evidence" style="display: block; font-weight: 600; color: #1a1a2e; margin-bottom: 0.4rem; font-size: 0.95rem;">
          Evidence links <span style="color: #888; font-weight: 400;">(optional)</span>
        </label>
        <textarea
          id="evidence"
          bind:value={evidence}
          rows={3}
          placeholder="Paste screenshot links, video URLs, or other evidence (one per line)..."
          style="width: 100%; padding: 0.65rem 0.9rem; border: 1px solid #ccc; border-radius: 6px; font-size: 1rem; box-sizing: border-box; resize: vertical;"
        ></textarea>
      </div>

      <button
        type="submit"
        disabled={submitting}
        style="
          padding: 0.75rem 2rem;
          background: #e94560;
          color: white;
          border: none;
          border-radius: 6px;
          font-size: 1rem;
          font-weight: 700;
          cursor: pointer;
          align-self: flex-start;
        "
      >
        {submitting ? 'Submitting…' : 'Submit Appeal'}
      </button>
    </form>
  {/if}
</div>
