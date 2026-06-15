# BetterModeration

> Smart cross-platform moderation for Discord and Minecraft.

## Features

- Unified ban, mute, kick, warn, and note system across Discord and Minecraft
- Cross-platform player profiles that link Discord accounts to Minecraft UUIDs
- Velocity proxy support: bans enforced at the network edge before players reach any backend server
- Paper plugin: login enforcement, chat mute checks, and all moderation commands
- Fabric mod: mixin-based chat interception and server lifecycle tracking
- Real-time event sync via Redis pub/sub
- Web dashboard for reviewing punishments, open reports, and appeals
- Public player lookup by Discord ID, Minecraft username, or UUID
- Self-serve appeal submission form

## Repository Structure

```
BetterModeration/
├── backend/                        # Go — REST API + Discord bot
│   ├── cmd/server/main.go          # Entry point
│   └── internal/
│       ├── api/router.go           # HTTP routes (net/http ServeMux)
│       ├── bot/bot.go              # discordgo bot wrapper
│       ├── cache/redis.go          # Redis cache wrapper
│       ├── db/postgres.go          # PostgreSQL pool wrapper
│       ├── domain/                 # Domain model structs
│       └── sync/eventbus.go        # Redis pub/sub event bus
│
├── web/                            # SvelteKit — TypeScript web frontend
│   └── src/routes/
│       ├── +page.svelte            # Home / landing page
│       ├── dashboard/              # Dashboard (auth-gated)
│       ├── lookup/                 # Public player lookup
│       └── appeal/                 # Appeal submission form
│
└── minecraft/
    ├── core/                       # Shared Java library (Maven)
    │   └── src/main/java/dev/bettermoderation/core/
    │       ├── api/                # BetterModerationAPI interface
    │       ├── domain/             # Punishment, Profile, enums
    │       └── sync/               # BackendEvent
    │
    ├── paper/                      # Paper plugin 1.21.4 (Maven + shade)
    │   └── src/main/java/dev/bettermoderation/paper/
    │       ├── BetterModerationPlugin.java
    │       ├── commands/           # ban, mute, kick, warn, link, report
    │       ├── listeners/          # PlayerLogin, PlayerChat, PlayerQuit
    │       └── sync/BackendClient.java
    │
    ├── velocity/                   # Velocity proxy plugin (Maven + shade)
    │   └── src/main/java/dev/bettermoderation/velocity/
    │       ├── BetterModerationVelocity.java
    │       ├── listeners/PlayerConnectListener.java
    │       └── sync/BackendClient.java
    │
    └── fabric/                     # Fabric mod 1.21.4 (Gradle / fabric-loom)
        └── src/main/java/dev/bettermoderation/fabric/
            ├── BetterModerationFabric.java
            ├── mixin/ChatMixin.java
            └── sync/BackendClient.java
```

## Quick Start

### Backend (Go)

```bash
cd backend
export DATABASE_URL="postgres://user:pass@localhost:5432/bettermoderation"
export REDIS_URL="redis://localhost:6379"
export DISCORD_TOKEN="your-bot-token"
go run ./cmd/server
```

### Web (SvelteKit)

```bash
cd web
npm install
npm run dev
```

### Minecraft — Core (shared library, build first)

```bash
cd minecraft/core
mvn install
```

### Minecraft — Paper plugin

```bash
cd minecraft/paper
mvn package
# Copy target/bettermoderation-paper-1.0.0-SNAPSHOT.jar to your Paper plugins folder
```

Configure `plugins/BetterModeration/config.yml`:
```yaml
backend-url: http://localhost:8080
server-id: my-survival
api-key: your-api-key
```

### Minecraft — Velocity proxy plugin

```bash
cd minecraft/velocity
mvn package
# Copy target/bettermoderation-velocity-1.0.0-SNAPSHOT.jar to your Velocity plugins folder
```

Set environment variables on the Velocity host:
```
BM_BACKEND_URL=http://localhost:8080
BM_SERVER_ID=velocity-proxy
BM_API_KEY=your-api-key
```

### Minecraft — Fabric mod

```bash
# Build the core library first (see above)
cd minecraft/fabric
./gradlew build
# Copy build/libs/bettermoderation-fabric-1.0.0.jar to your Fabric mods folder
```

Set environment variables on the Fabric server:
```
BM_BACKEND_URL=http://localhost:8080
BM_SERVER_ID=fabric-server
BM_API_KEY=your-api-key
```
