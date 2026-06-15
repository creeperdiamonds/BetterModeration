<script lang="ts">
  // Layout component — wraps all pages with a top nav bar
</script>

<nav style="
  display: flex;
  align-items: center;
  gap: 2rem;
  padding: 0.75rem 2rem;
  background: #1a1a2e;
  color: #e0e0e0;
  font-family: system-ui, sans-serif;
  border-bottom: 2px solid #e94560;
">
  <span style="font-weight: 700; font-size: 1.2rem; color: #e94560; margin-right: 1rem;">
    BetterModeration
  </span>
  <a href="/" style="color: #e0e0e0; text-decoration: none; font-size: 0.95rem;">Home</a>
  <a href="/dashboard" style="color: #e0e0e0; text-decoration: none; font-size: 0.95rem;">Dashboard</a>
  <a href="/lookup" style="color: #e0e0e0; text-decoration: none; font-size: 0.95rem;">Player Lookup</a>
  <a href="/appeal" style="color: #e0e0e0; text-decoration: none; font-size: 0.95rem;">Submit Appeal</a>
</nav>

<main style="font-family: system-ui, sans-serif; color: #1a1a2e; min-height: calc(100vh - 56px);">
  <slot />
</main>
