# Integrações de produção

A intranet oferece eventos versionados por projeto, armazenamento privado de arquivos e exportação de créditos aprovados. O cliente comum está em `integrations/cli/studio_client.py`, usa Python 3.8+ e a biblioteca padrão.

## Configuração

Defina no ambiente do processo `STUDIO_URL` com a URL GraphQL da intranet, `STUDIO_PROJECT_ID` com o projeto e a autorização existente em `STUDIO_AUTHORIZATION` ou `STUDIO_COOKIE`. Os valores de autorização permanecem no ambiente. O cliente não os grava em eventos, arquivos de configuração ou logs.

Para chaves da API Wiki.js, habilite a API conforme a configuração existente e associe o grupo da chave ao projeto na área Acesso das integrações. O papel Colaborador pode enviar artefatos, builds e resultados. Gestores humanos revisam e aprovam. Chaves com permissão administrativa seguem o escopo administrativo existente. As ações de serviço registram `api:<identificador>` no histórico.

`STUDIO_OUTBOX` escolhe o diretório da fila local. Cada evento possui ID estável, origem e conteúdo. `flush` reenvia eventos pendentes. Conflitos de revisão são resolvidos com nova leitura do projeto. O servidor rejeita reutilização do ID com conteúdo diferente.

## Cliente comum

```text
python integrations/cli/studio_client.py tasks
python integrations/cli/studio_client.py artifact dialogue.gdkg --source goethe --artifact-id ARTIFACT_ID --version 1 --stage engine_integration --tool-uri "goethe://open?repo=GhostLore&path=dialogues/intro.yaml"
python integrations/cli/studio_client.py stage dlg-intro polish --source goethe --vcs-revision main@42
python integrations/cli/studio_client.py vcs-sync D:/roguefairy/GhostLore --slug ghostlore
python integrations/cli/studio_client.py credits SNAPSHOT_ID credits.json
python integrations/cli/studio_client.py build game.zip --source carnifex --release-id RELEASE_ID --snapshot-id SNAPSHOT_ID --name build-001 --vcs-revision a1b2c3d4
python integrations/cli/studio_client.py event event.json
python integrations/cli/studio_client.py flush --force
```

## Sincronização de status Git & Lore (Epic Unreal Revision Control)

O subcomando `vcs-sync` detecta automaticamente repositórios **Git** (`.git`), **Lore** (`.lore` — implementação open-source do Unreal Revision Control da Epic Games em `lores://` / `lore://`) e workspaces **UEFN / URC** (`*.uefnproject` / `.urc`), enviando eventos `vcsStatus` idempotentes para o projeto:

- **Git**: branch atual, commit `SHA`, mensagem/autor, contadores `ahead`/`behind`, arquivos modificados/staged e remoção automática de credenciais em `remote.origin.url`.
- **Lore (`lore --json`)**: consome os fluxos NDJSON de `lore --repository <dir> --json revision info`, `status` e `lock query`, publicando revisão `branch@N` (assinatura BLAKE3), estado de compressão (`zstd`), arquivos binários modificados localmente e **locks exclusivos ativos** (`owner`, `path`, `branch`, `lockedAt`).
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

## Geppetto / Krita

Copie `integrations/geppetto/rogue_studio` e `rogue_studio.desktop` para a pasta de recursos `pykrita` do editor. Habilite Rogue Fairy Studio no gerenciador de plugins Python e reinicie o editor.

Defina `STUDIO_CLIENT_PATH` com o caminho absoluto do cliente Python, `STUDIO_PYTHON` com o executável Python e `STUDIO_ARTIFACT_ID` com o artefato de destino. O menu Tools / Scripts contém Publish artwork to Studio. A ação exige documento salvo, exporta um preview PNG e publica a fonte editável e o preview pela fila comum.

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
