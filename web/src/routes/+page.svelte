<script lang="ts">
  const steps = [
    {
      step: '1',
      title: 'Invite the Bot',
      description: 'Add the BetterModeration bot to your Discord server using the official invite link.'
    },
    {
      step: '2',
      title: 'Get Your Server Code',
      description: 'Run /bmsetup in your Discord server. The bot will give you a unique server code.'
    },
    {
      step: '3',
      title: 'Link Your Minecraft Server',
      description: 'Install the plugin/mod on your Minecraft server and run /link <code> to pair it with Discord.'
    }
  ];
</script>

<svelte:head>
  <title>BetterModeration — Cross-Platform Moderation</title>
</svelte:head>

<div style="max-width: 860px; margin: 0 auto; padding: 4rem 2rem;">
  <h1 style="font-size: 2.8rem; font-weight: 800; color: #1a1a2e; margin-bottom: 0.5rem;">
    BetterModeration
  </h1>
  <p style="font-size: 1.3rem; color: #e94560; font-weight: 600; margin-bottom: 1.5rem;">
    Smart cross-platform moderation for Discord and Minecraft
  </p>
  <p style="font-size: 1rem; line-height: 1.7; color: #444; max-width: 640px; margin-bottom: 3rem;">
    BetterModeration unifies your moderation workflow across Discord and Minecraft. Issue bans, mutes,
    kicks, and warnings from a single system that syncs instantly across all your platforms. Every
    action is logged, auditable, and accessible from the web dashboard.
  </p>

  <h2 style="font-size: 1.6rem; font-weight: 700; color: #1a1a2e; margin-bottom: 1.5rem;">
    Get Started
  </h2>

  <div style="display: flex; flex-direction: column; gap: 1.25rem;">
    {#each steps as item}
      <div style="
        display: flex;
        align-items: flex-start;
        gap: 1.25rem;
        background: #f5f5f5;
        border-radius: 8px;
        padding: 1.25rem 1.5rem;
        border-left: 4px solid #e94560;
      ">
        <span style="
          background: #e94560;
          color: white;
          font-weight: 700;
          font-size: 1rem;
          border-radius: 50%;
          width: 2rem;
          height: 2rem;
          display: flex;
          align-items: center;
          justify-content: center;
          flex-shrink: 0;
        ">{item.step}</span>
        <div>
          <strong style="display: block; color: #1a1a2e; margin-bottom: 0.25rem;">{item.title}</strong>
          <span style="color: #555; font-size: 0.95rem;">{item.description}</span>
        </div>
      </div>
    {/each}
  </div>
</div>
