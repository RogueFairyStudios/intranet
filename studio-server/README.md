# Rogue Fairy Studio Server (`studio-server`)

Servidor Go dedicado à **Gestão de Projetos & Integração Cross-Tool** da **Rogue Fairy Studios**, operando como hub central entre:

- **Rogue Fairy Intranet** (`https://docs.roguefairy.net/p/studio`, Wiki.js delegando via `STUDIO_API_URL` + `STUDIO_INTERNAL_KEY`)
- **Tellurian Authentik** (`https://auth.telluriancorp.info` — validação OIDC multi-aplicação via JWKS + consulta opcional à Core API `/api/v3/core/users/`)
- **Aetherion** (engine baseada em Godot — aba **Studio** no Project Manager consultando `/v1/me/projects?client=aetherion` com o Bearer token OIDC da conta Authentik)
- **Geppetto** (editor de arte 2D baseado em Krita — plugin `rogue_studio` consultando `/v1/me/workspaces?client=geppetto&vcs=lore` para listar projetos, repositórios Lore de arte, locks `.kra`, artefatos `art_2d` e cápsulas de lojas)
- **Lore VCS** (`lores://lore.roguefairy.net/<project>/<repo>`) e demais ferramentas do estúdio (`goethe`, `animafaber`, `fontpit`, `carnifex`, CI).

## Endpoints Principais

| Método | Rota | Descrição |
|---|---|---|
| `GET` | `/healthz`, `/v1/health` | Healthcheck do serviço e conexão PostgreSQL |
| `GET` | `/v1/auth/me` | Identidade resolvida (Authentik OIDC, grupos, vínculo Wiki.js e ferramenta inferida) |
| `GET` | `/v1/me/projects` | Lista projetos acessíveis ao usuário autenticado (ex.: `?client=aetherion` para o Project Manager) |
| `GET` | `/v1/me/workspaces` | Lista workspaces acionáveis, repositórios Lore/Git, tasks atribuídas, artefatos e slots de loja (ex.: `?client=geppetto&vcs=lore&onlyWritable=1`) |
| `GET` | `/v1/users/{identifier}/projects` | Responde *"Usuário X está em quais projetos?"* por ID Wiki, `sub`, e-mail ou username Authentik |
| `GET` | `/v1/users/{identifier}/workspaces` | Responde *"Em qual projeto e repositório Lore o Usuário Y deve trabalhar na arte?"* |
| `GET` | `/v1/configs` | Catálogo de perfis generalistas de configuração do estúdio (`rfs.studio.config/v1`) |
| `GET` | `/v1/configs/{tool}` | Gera manifesto **JSON** ou **YAML** generalista do estúdio para `{tool}` (`aetherion`, `geppetto`, `lore`, `authentik`, `goethe`, `carnifex`, `animafaber`, `fontpit`, `uri-handler`, `all`) |
| `GET` | `/v1/projects/{projectRef}/configs` | Catálogo de perfis de configuração específicos do projeto (aceita `UUID` ou `slug`, ex.: `klaus-syndicate`) |
| `GET` | `/v1/projects/{projectRef}/configs/{tool}` | Gera manifesto **JSON** ou **YAML** do projeto para `{tool}` (engine, Authentik, Lore, repositórios, locks `.kra`, permissões efetivas e workspaces) |
| `GET` | `/v1/projects` | Lista agregados completos de projetos com redação automática de ACLs para não-managers |
| `POST` | `/v1/projects` | Cria novo projeto (`digital`, `analogue`, `hybrid`) com `engine` (`aetherion`, `godot`, `carnifex`, `uefn`, `unity`, `tabletop`, `custom`) |
| `GET` | `/v1/projects/{projectRef}` | Obtém estado, revisão e permissões do projeto (por `UUID` ou `slug`) |
| `POST` | `/v1/projects/{projectId}/commands` | Executa comandos de domínio com controle de concorrência otimista (`revision`) |
| `POST` | `/v1/projects/{projectId}/events` | Ingestão idempotente de eventos de ferramentas (`artifact`, `artifactVersion`, `artifactStage`, `vcsStatus`, `build`, `validation`) com retry CAS automático |
| `GET` | `/v1/projects/{projectId}/credits/{snapshotId}` | Exporta créditos aprovados em `json` ou `markdown` |
| `POST` | `/v1/projects/{projectId}/files` | Upload binário privado deduplicado por SHA-256 (`multipart/form-data`) |
| `GET` | `/v1/projects/{projectId}/files/{fileId}` | Download autenticado de binários privados |
| `GET/POST/DELETE` | `/v1/service-keys` | Gestão administrativa de Service Keys `stk_...` (`X-Studio-Key`) |

## Negociação de Formato nos Endpoints de Configuração (`/v1/configs`)

Todos os endpoints de configuração suportam **JSON** e **YAML** nativamente:
- Query string `?format=yaml` ou `?format=json`
- Sufixo na rota (ex.: `/v1/configs/aetherion.yaml`, `/v1/projects/klaus-syndicate/configs/geppetto.json`)
- Header HTTP `Accept: application/yaml` (ou `text/yaml`)
- `?download=1` adiciona `Content-Disposition: attachment; filename="<escopo>-<tool>.<ext>"`
- `?user=<identifier>` resolve permissões efetivas de repositórios/Lore para um colaborador específico

Variáveis opcionais para ajustar os endpoints emitidos nos manifestos:
- `STUDIO_PUBLIC_URL` (padrão: `https://studio.roguefairy.net`)
- `STUDIO_INTRANET_URL` (padrão: `https://docs.roguefairy.net`)
- `STUDIO_AUTHENTIK_URL` (padrão: `https://auth.telluriancorp.info`)
- `STUDIO_LORE_SERVER_URL` (padrão: `lores://lore.roguefairy.net`)
- `STUDIO_OIDC_CLIENT_IDS` (mapa opcional `slug=client_id,...` para customizar `clientId` por ferramenta)

## Execução Local e Testes

```bash
cd studio-server
go test -race ./...
go run ./cmd/studio-server
```

## Deploy no Railway

O diretório `studio-server/` possui `Dockerfile` multi-stage (`distroless:nonroot`) e `railway.json` próprios. Configure um serviço no projeto `roguefairy` com `rootDirectory = studio-server` compartilhando `DATABASE_URL` do `Postgres` e defina `STUDIO_API_URL` + `STUDIO_INTERNAL_KEY` no serviço `intranet`.
