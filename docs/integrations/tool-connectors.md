# Integrações de produção

A intranet oferece eventos versionados por projeto, armazenamento privado de arquivos e exportação de créditos aprovados. O cliente comum está em `integrations/cli/studio_client.py`, usa Python 3.8+ e a biblioteca padrão.

## Configuração

O cliente `integrations/cli/studio_client.py` suporta dois transportes:

1. **Go Studio Server REST `/v1` (recomendado para Aetherion, Geppetto, Lore CLI e automações multi-ferramenta)**:
   - Defina `STUDIO_API_URL` (ex.: `https://studio.roguefairy.net` ou `http://127.0.0.1:8090` em desenvolvimento local).
   - Autentique interativamente via **Tellurian Authentik OIDC Device Code (RFC 8628)** com `studio_client.py login` (sessão salva com permissão restrita `0600` e renovação automática de `refresh_token`), informe `STUDIO_AUTHORIZATION="Bearer <jwt>"` ou utilize uma chave de serviço em `STUDIO_KEY="stk_..."`.
2. **Wiki.js GraphQL (`/graphql` — compatibilidade direta)**:
   - Defina `STUDIO_URL` com a URL GraphQL da intranet, `STUDIO_PROJECT_ID` com o projeto e `STUDIO_AUTHORIZATION` ou `STUDIO_COOKIE`.

Os valores de autorização permanecem no ambiente ou no arquivo de sessão OIDC do usuário; o cliente nunca os grava em eventos da fila (`outbox`), arquivos de projeto ou logs.

Para chaves da API Wiki.js ou chaves `stk_...` do `studio-server`, o papel Colaborador (`editor`) pode enviar artefatos, builds e resultados, enquanto gestores humanos revisam e aprovam.

`STUDIO_OUTBOX` escolhe o diretório da fila local. Cada evento possui ID estável, origem e conteúdo. `flush` reenvia eventos pendentes. Conflitos de revisão são resolvidos com nova leitura do projeto. O servidor rejeita reutilização do ID com conteúdo diferente.

## Cliente comum e consultas transversais

```text
# Login OIDC Device Code no Tellurian Authentik (para Geppetto, CLI, Goethe, etc.)
python integrations/cli/studio_client.py login --issuer https://auth.telluriancorp.info/application/o/geppetto/ --client-id geppetto
python integrations/cli/studio_client.py whoami

# Consultas de projetos e workspaces Lore por usuário / ferramenta
python integrations/cli/studio_client.py my-projects --client aetherion
python integrations/cli/studio_client.py user-projects alice@roguefairy.net --client aetherion
python integrations/cli/studio_client.py my-work --client geppetto --vcs lore --only-writable
python integrations/cli/studio_client.py user-work bianca@roguefairy.net --client geppetto --vcs lore

# Exportação de manifestos JSON / YAML de configuração (escopo estúdio ou projeto)
python integrations/cli/studio_client.py config --scope studio
python integrations/cli/studio_client.py config aetherion --scope studio --format yaml -o roguefairy-aetherion.yaml
python integrations/cli/studio_client.py config geppetto --scope studio --format yaml -o roguefairy-geppetto.yaml
python integrations/cli/studio_client.py config aetherion --project klaus-syndicate --format yaml -o klaus-syndicate-aetherion.yaml
python integrations/cli/studio_client.py config geppetto --project klaus-syndicate --format json -o klaus-syndicate-geppetto.json

# Operações de produção por projeto (funcionam tanto via STUDIO_API_URL quanto via STUDIO_URL)
python integrations/cli/studio_client.py tasks
python integrations/cli/studio_client.py artifact dialogue.gdkg --source goethe --artifact-id ARTIFACT_ID --version 1 --stage engine_integration --tool-uri "goethe://open?repo=GhostLore&path=dialogues/intro.yaml"
python integrations/cli/studio_client.py stage dlg-intro polish --source goethe --vcs-revision main@42
python integrations/cli/studio_client.py vcs-sync D:/roguefairy/GhostLore --slug ghostlore
python integrations/cli/studio_client.py credits SNAPSHOT_ID credits.json
python integrations/cli/studio_client.py build game.zip --source carnifex --release-id RELEASE_ID --snapshot-id SNAPSHOT_ID --name build-001 --vcs-revision a1b2c3d4
python integrations/cli/studio_client.py event event.json
python integrations/cli/studio_client.py flush --force
```

## Manifestos de Configuração das Ferramentas & Estações (`rfs.studio.config/v1` — JSON / YAML)

Tanto o `studio-server` (`/v1/configs/...` e `/v1/projects/{projectRef}/configs/...`) quanto a Intranet (`/p/studio/configs/...` e `/p/studio/projects/:projectRef/configs/...`) fornecem arquivos de configuração padronizados em **JSON** e **YAML** (`schemaVersion: "rfs.studio.config/v1"`) para todas as ferramentas do estúdio (`aetherion`, `geppetto`, `lore`, `authentik`, `goethe`, `carnifex`, `animafaber`, `fontpit`, `uri-handler` e `all`):

1. **Configurações Generalistas do Estúdio (`scope: "studio"`)**:
   - **`GET /v1/configs/aetherion?format=yaml`** (ou `/v1/configs/authentik`): fornece `editorSettings` (`network/authentik/issuer_url`, `client_id`, `scopes`, `studio_api_url`, `persist_session`, `auto_refresh`), variáveis de ambiente e repositório da engine Aetherion apontando para o **Tellurian Authentik** e o **Studio Server** da Rogue Fairy Studios.
   - **`GET /v1/configs/geppetto?format=yaml`**: configura globalmente o Geppetto da equipe de arte apontando para o servidor **Tellurian Authentik** (`https://auth.telluriancorp.info`) e para o servidor **Lore VCS** (`lores://lore.roguefairy.net`), incluindo `kritaVersioningSettings` (`[krita_versioning]`) e variáveis do plugin `rogue_studio`.
2. **Configurações Específicas por Projeto (`scope: "project"`, por UUID ou slug como `klaus-syndicate`)**:
   - **Desenvolvedor na engine Aetherion (`GET /v1/projects/klaus-syndicate/configs/aetherion?format=yaml`)**: para um projeto como *"Klaus Syndicate"* (`engine: "aetherion"`), entrega metadados do projeto (`usesAetherion: true`, `aetherionUri`), `editorSettings`, `projectSettings` (`application/config/name: "Klaus Syndicate"`, `network/authentik/*`), `workspace` (`primaryWorkspacePath`, `bootstrapCommands`), lista de repositórios Git e Lore com comandos de clone e `access` efetivo do desenvolvedor.
   - **Artistas e Animadores no Geppetto (`GET /v1/projects/klaus-syndicate/configs/geppetto?format=yaml`)**: além dos endereços gerais do Tellurian Authentik e do Lore, entrega o namespace Lore do projeto (`klaus-syndicate/*`), `kritaVersioningSettings.projectEntry` (`localWorkspaceRoot`, `accessLevel`, `writable`, `canCreateRepo`, `allowedActions`, pastas de arte e animação), repositórios `lores://lore.roguefairy.net/klaus-syndicate/...` com locks binários `.kra` ativos, `access` efetivo (papel no projeto, permissões Lore e grants de grupos Authentik), `artPipeline.artifacts` e `artPipeline.storefrontSlots`.

## Sincronização de status Git & Lore (Epic Unreal Revision Control)

O subcomando `vcs-sync` detecta automaticamente repositórios **Git** (`.git`), **Lore** (`.lore` — implementação open-source do Unreal Revision Control da Epic Games em `lores://` / `lore://`) e workspaces **UEFN / URC** (`*.uefnproject` / `.urc`), enviando eventos `vcsStatus` idempotentes para o projeto:

- **Git**: branch atual, commit `SHA`, mensagem/autor, contadores `ahead`/`behind`, arquivos modificados/staged e remoção automática de credenciais em `remote.origin.url`.
- **Lore (`lore --json`)**: consome os fluxos NDJSON de `lore --repository <dir> --json revision info`, `status` e `lock query`, extraindo automaticamente o namespace `<project>/<repo>` de `remote_url` em `.lore/config.toml` (ex.: `lores://lore.roguefairy.net/ghostlore/client` → `loreProject: ghostlore`, `loreRepo: client`), publicando revisão `branch@N` (assinatura BLAKE3), estado de compressão (`zstd`), arquivos binários modificados localmente e **locks exclusivos ativos** (`owner`, `path`, `branch`, `lockedAt`).
- **UEFN / Epic URC**: lê metadados de `*.uefnproject` (`bindings.projectId`) e presença de `.urc` (`oodle`), vinculando a URI `uefn://`.

Pode ser executado manualmente por artistas/engenheiros, em *git hooks* (`post-commit`, `post-merge`) ou em jobs de CI.

## Handlers de URI das ferramentas (`rogue_uri_handler.py`)

O despachante multiplataforma `integrations/uri_handler/rogue_uri_handler.py` registra os protocolos customizados do estúdio no escopo do usuário (`HKCU\Software\Classes` no Windows e `~/.local/share/applications/*.desktop` via `xdg-mime` no Linux), permitindo abrir assets diretamente a partir de `/p/studio`:

```text
python integrations/uri_handler/rogue_uri_handler.py install --workspace-root D:/roguefairy
python integrations/uri_handler/rogue_uri_handler.py doctor
python integrations/uri_handler/rogue_uri_handler.py open "goethe://open?repo=GhostLore&path=dialogues/intro.yaml" --dry-run
```

Protocolos suportados:
- `aetherion://open?project=<dir>[&scene=res://cena.tscn][&debug=1]` → abre o projeto no editor Aetherion
- `goethe://open?repo=<repo>&path=<arquivo.yaml>[&node=<id>]` → `goethe_imgui_demo` (ou VS Code para `.gproj.yaml`)
- `geppetto://open?repo=<repo>&path=<arte.kra>[&workspace=<ws>][&layer=<camada>]` → `krita --nosplash -- <arquivo>`
- `godot://open?project=<dir>[&scene=res://cena.tscn][&debug=1]` → `godot --path <dir> -e`
- `carnifex://open?game=<dir>[&mod=<mod>][&map=<mapa>]` → `carnifex -basedir <dir> -window +map <mapa>`
- `lore://open?repo=<repo>[&path=<asset>][&action=reveal|sync]` → revela no gerenciador de arquivos ou executa `lore sync --root-file`
- `uefn://open?project=<dir_ou_uefnproject>[&verse=1]` → abre no UEFN ou workspace Verse
- `animafaber://open?spirit=<id>` → abre o espírito no AnimaFaber (`https://`)
- `fontpit://open?family=<slug>[&mode=web|cli]` → inspeciona a família tipográfica no Fontpit

Por segurança, o despachante valida contenção estrita de caminhos dentro de `workspaceRoot`, bloqueia *path traversal* (`..`), rejeita argumentos iniciados por `-` e nunca invoca `shell=True`.

Crie o artefato na intranet para obter seu ID. Integrações também podem registrar artefatos com o tipo de evento `artifact`, contendo `name`, `kind` e `externalId`. Eventos `artifactVersion` posteriores podem identificar esse registro por `externalId` e origem.

Arquivos locais são enviados para armazenamento privado no banco da intranet antes de registrar a versão. Acesso aos downloads exige acesso ao projeto. Uploads respeitam o limite configurado no Wiki.js e o teto de 50 MiB deste conector. Para pacotes maiores, envie um evento com URI HTTPS do armazenamento de distribuição e SHA-256. O conector registra essa referência externa, cujo controle de acesso é responsabilidade do armazenamento escolhido.

Cada versão de artefato e build tem identidade imutável. Use uma nova versão ou nome quando os bytes mudarem. O nome de build já registrado rejeita conteúdo ou créditos diferentes.

Eventos de validação usam `name`, `kind` (`test` ou `playtest`), `result` (`passed`, `failed` ou `pending`), `notes` e referências opcionais `releaseId`, `artifactVersionId` e `taskId`.

## Aetherion (Project Manager — aba nativa `Studio`)

O fork **Aetherion** (`modules/authentik` + `editor/project_manager`) possui integração nativa com o Tellurian Authentik e o `studio-server`:

1. Abra o **Project Manager** do Aetherion e clique no botão **Sign In** (canto superior direito) ou na aba **Studio**.
2. Em **OIDC / Authentik Provider Settings**, configure manualmente ou clique em **Import Studio Config (.json)...** para importar diretamente um manifesto exportado pelo Studio Server (`roguefairy-aetherion.json` ou `<projeto>-aetherion.json`):
   - **Issuer URL**: `https://auth.telluriancorp.info/application/o/aetherion/`
   - **Client ID**: `<client_id do provider Aetherion>`
   - **Studio API URL**: `https://studio.roguefairy.net` (ou defina `AETHERION_STUDIO_API_URL` / `STUDIO_API_URL` no ambiente).
3. Após autenticar via navegador (PKCE) ou Device Code, a aba **Studio** consulta `GET /v1/me/projects?client=aetherion` (ou `GET /v1/users/{id}/projects` ao pesquisar outro colaborador no campo *User*) e exibe:
   - Todos os projetos digitais/híbridos acessíveis ao usuário, engine alvo (`Aetherion`, `Godot`, `Carnifex`, etc.), fase atual, marco/sprint ativo, papel no estúdio e contagem de tarefas atribuídas.
   - Namespace **Lore VCS** (`<projectSlug>/*`), permissões efetivas (`read`, `write`, `canCreateRepo`) e lista de repositórios **Git** e **Lore** com contagem de locks.
   - Ações diretas: **Clone Selected Repository...** (`git clone` / `lore clone` sem shell), **Bind Local Folder...** (persistido em `authentik_studio_bindings.cfg` e registrado na lista local do Project Manager), **Open in Aetherion**, **Export Studio Config...** e **Export Project Config...** (salvando `<slug>-aetherion.yaml` ou `.json` na máquina do desenvolvedor).

Scripts de ferramentas/automação dentro do Aetherion também podem instanciar `Authentik.create_studio_client()` (`StudioApiClient`) para consultar `fetch_my_projects()`, `fetch_my_workspaces("lore")`, `fetch_studio_config("aetherion", "yaml")` e `fetch_project_config("klaus-syndicate", "aetherion", "json")`.

## Geppetto / Krita

Copie `integrations/geppetto/rogue_studio` e `rogue_studio.desktop` para a pasta de recursos `pykrita` do editor. Habilite **Rogue Fairy Studio** no gerenciador de plugins Python e reinicie o editor.

Defina `STUDIO_CLIENT_PATH` com o caminho absoluto de `studio_client.py`, `STUDIO_PYTHON` com o executável Python e `STUDIO_API_URL` com a URL do `studio-server` (após autenticar com `studio_client.py login` ou `STUDIO_KEY`). O menu **Tools / Scripts** oferece duas ações:

- **Studio Projects & Lore Art...**: consulta `my-work --client geppetto --vcs lore`, listando em quais projetos o artista trabalha, quais repositórios **Lore** (`lores://lore.roguefairy.net/<project>/<repo>`) possuem permissão de escrita, comandos `lore clone` copiáveis, locks ativos em `.kra`, tarefas/artefatos `art_2d` e slots pendentes de cápsulas gráficas das lojas, permitindo selecionar o projeto e artefato alvo diretamente na interface e exportar manifestos **YAML/JSON** pelos botões **Studio Config (YAML/JSON)...** (configuração global do Geppetto + Authentik + Lore) e **Export Project Config...** (configuração específica do projeto selecionado, ex.: `klaus-syndicate-geppetto.yaml`).
- **Publish artwork to Studio**: exige documento salvo, exporta um preview PNG e publica o arquivo editável (`.kra`) e o preview para o projeto/artefato selecionado (ou definido em `STUDIO_PROJECT_ID` / `STUDIO_ARTIFACT_ID`).

## Godot

Copie `integrations/godot/addons/rogue_studio` para `addons/rogue_studio` no projeto e habilite o plugin. Defina `STUDIO_CLIENT_PATH`, `STUDIO_PYTHON` e as variáveis comuns antes de iniciar o editor.

O menu Tools oferece importação de créditos para `res://credits.json`, publicação do recurso selecionado e publicação de build. A importação usa `STUDIO_SNAPSHOT_ID`. Recursos usam `STUDIO_ARTIFACT_ID`. Builds usam `STUDIO_BUILD_PATH`, `STUDIO_RELEASE_ID`, `STUDIO_SNAPSHOT_ID` e `STUDIO_BUILD_NAME`. Operações de rede executam em uma thread para manter a interface responsiva.

## Unity

Copie `integrations/unity/Editor/RogueStudio.cs` para uma pasta `Editor` do projeto. O menu Tools / Rogue Fairy Studio oferece as mesmas operações. Créditos são importados para `Assets/StreamingAssets/credits.json`. O processo Python executa fora da thread principal. As variáveis de ambiente são as mesmas do adaptador Godot.

## Goethe, Carnifex e outras engines

Inclua os comandos do cliente após a exportação de pacotes `.gdkg`, exportações de narrativa ou empacotamento da build. Exporte os créditos aprovados antes de empacotar o jogo e registre a build com o mesmo snapshot. Goethe usa `--source goethe`, Carnifex usa `--source carnifex` e outras ferramentas usam `--source generic`.

O código de saída `0` indica envio concluído, `2` indica eventos pendentes na fila e `1` indica falha de configuração ou operação. A automação pode repetir `flush` após recuperar conectividade. Se um arquivo local foi alterado ou removido, preserve o evento original para diagnóstico e crie um novo evento para a nova versão. Outros eventos continuam sendo processados.

## Verificações realizadas

O cliente passou por testes de conflito, reenvio, checksum, caminhos e falha isolada de arquivo. O transporte foi exercitado contra HTTP local e PostgreSQL 16, incluindo upload/download privado e exportação de créditos. O plugin Godot carregou no editor 4.3 em modo headless. A ativação nas instalações reais de Geppetto e Unity ainda depende dos respectivos editores e da configuração de acesso do estúdio.
