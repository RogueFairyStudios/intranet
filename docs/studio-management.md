# Gestão de produção na intranet

Abra Projetos no cabeçalho da intranet ou acesse `/p/studio`. Visitantes não autenticados são redirecionados automaticamente para o SSO (Tellurian Authentik) e retornam ao destino após o login. Um administrador cria o projeto como digital, analógico ou híbrido e concede acesso aos gestores e colaboradores.

A seção inicial **Visão geral** consolida marco atual, ciclo ativo, tarefas abertas/concluídas, participantes ativos, progresso de integração de assets na engine, saúde dos repositórios VCS (Git e Lore/Epic URC), bloqueios em aberto, revisões pendentes, documentos desatualizados e últimas entregas, com links profundos preservados na URL (`?project=<id>&tab=<secao>&lang=<idioma>`).

## Pipeline de desenvolvimento, URIs de ferramentas e VCS (Git & Lore)

A seção **Pipeline** reflete as duas camadas das pipelines modernas de jogos:

1. **Fases macro do projeto (`pre_production`, `production`, `post_production`, `live`)**: acompanha em que estágio macro o projeto se encontra, quantos marcos e tarefas pertencem a cada fase e permite aos gestores avançar ou ajustar a fase ativa do projeto.
2. **Matriz não-linear de pipeline de assets (`concept`, `blockout`, `authoring`, `surface_loc`, `engine_integration`, `polish`)**: cada artefato/asset é classificado por disciplina (*track*), ferramenta do estúdio (`Aetherion`, `Goethe`, `Geppetto`, `Godot`, `Carnifex`, `AnimaFaber`, `Fontpit`, `Lore`, `UEFN`, etc.), caminho no repositório e URI direta de lançamento (`aetherion://`, `goethe://`, `geppetto://`, `godot://`, `carnifex://`, `lore://`, `uefn://`, `animafaber://`, `fontpit://`). Qualquer célula da matriz pode ser acionada para avançar **ou retornar** um asset de forma não-linear após testes na engine ou revisão de direção.
3. **Versionamento Git & Lore (Epic Unreal Revision Control)**: registra os repositórios vinculados ao projeto (`git`, `lore`, `urc`), aplicando o namespace por projeto `<project>/<repo>` (`lores://lore.roguefairy.net/<project>/<repo>`) integrado ao `rfs-lore-bridge` e ao **Tellurian Authentik** (`https://auth.telluriancorp.info`). Exibe branch/stream atual, revisão (`SHA` ou `branch@N` BLAKE3), estado da working tree, comandos prontos da CLI (`lore auth login`, `lore repository create`, `lore clone`) e **locks exclusivos de arquivos binários** do Lore/URC correlacionados diretamente nas linhas da matriz de assets.
4. **Gerador de Configurações das Ferramentas & Estações (`JSON` / `YAML` — `rfs.studio.config/v1`)**: cada projeto declara sua engine/runtime alvo (`aetherion`, `godot`, `carnifex`, `uefn`, `unity`, `tabletop`, `custom`) e disponibiliza na aba **Pipeline** visualização ao vivo, cópia e download de manifestos `.yaml` e `.json` tanto **generalistas do estúdio** (`/p/studio/configs/:tool`) quanto **específicos do projeto** (`/p/studio/projects/:projectRef/configs/:tool`) para `Aetherion`, `Geppetto`, `Lore`, `Tellurian Authentik`, `Goethe`, `Carnifex`, `AnimaFaber`, `Fontpit`, `URI Handlers` e estação completa (`all`).

## Equipe, grupos do Tellurian Authentik e permissões Lore

Cadastre equipes e participantes. Pessoas externas podem ser incluídas sem conta de acesso. Cada participante possui nome público, funções nos créditos, equipe e período de participação. O vínculo opcional com uma conta facilita identificar membros internos.

O controle de acesso combina vínculos individuais e **grupos sincronizados do Tellurian Authentik**:

- **`Rogue Fairy Studios - C-Level` e `authentik Admins`**: administração total (`manager`) em todos os projetos, inclusive projetos com visibilidade `restricted`.
- **`Rogue Fairy Studios` (funcionários)**: em projetos com `lore.visibility = internal`, gestores podem definir o papel padrão do estúdio no painel (`none`, `reader` ou `editor`). Mudar a visibilidade para `restricted` bloqueia o acesso automático e exige confirmação explícita antes de reabrir o projeto para `internal`.
- **`Rogue Fairy Studios - Partners` (parceiros externos)**: isolamento *fail-closed* — membros desse grupo **nunca** herdam acesso padrão do estúdio (mesmo em caso de dupla associação acidental) e só acessam projetos onde um grupo específico do projeto (ex.: `Rogue Fairy Studios - GhostLore - Partners`) ou sua conta individual foi vinculado.
- **Atributos nativos do Authentik e Blueprint YAML**: cada grupo vinculado em **Equipe** define o papel no painel (`reader`, `editor`, `manager`) e as permissões Lore (`lore_actions` entre `read`, `write`, `owner`, `admin`, `migrate`, `push-protected`, `obliterate`, `presign` e `lore_can_create_repo`), gerando diretamente na interface o JSON de *custom attributes* e o Blueprint v1 YAML sob `Rogue Fairy Studios - Lore Projects`.

Encerrar uma participação preserva tarefas históricas e créditos. Arquivar uma equipe preserva seus membros históricos. Arquivar o projeto suspende alterações de produção até a reativação por um gestor.

## Documentação e planejamento

Em Documentos, escolha um modelo, título, caminho e idioma para gerar um rascunho no editor Markdown nativo da wiki. Os modelos têm versão identificada. Também é possível buscar e vincular páginas existentes. As permissões da página continuam sendo aplicadas. Quando a página muda, a referência mostra a necessidade de revisão e pode ser atualizada deliberadamente.

Cadastre marcos com critérios de aceite e ciclos com objetivos, períodos e capacidade. As tarefas podem indicar responsável, disciplina, prioridade, prazo, bloqueio, dependências, marco, ciclo e documentos relacionados. Lista e quadro mostram os mesmos registros. Dependências circulares são rejeitadas. A conclusão exige resolver bloqueios e dependências abertas.

## Produção e validação

Cadastre artefatos associados às tarefas e documentos. Cada versão possui localização e SHA-256 e permanece imutável — na interface web, você pode enviar o arquivo diretamente para o armazenamento privado do projeto (`/p/studio/files/:projectId`), preenchendo URL e SHA-256 automaticamente, ou informar uma URI externa. Gestores registram revisão e aprovação de versões específicas. O histórico conserva as decisões anteriores.

Registre testes e playtests com resultado, observações e referências à tarefa, entrega ou versão avaliada. Jogos analógicos podem usar artefatos de cartas, regras, tabuleiros, componentes e arquivos de impressão no mesmo fluxo.

## Pós-produção: Lojas, Conformidade & Publicação (`Lojas & Conformidade`)

Na fase de **Pós-produção** (acessível pela aba **Lojas & Conformidade** ou pelo atalho direto no card de Pós-produção da Pipeline), o projeto organiza a preparação por plataforma/loja de destino:

- **Presets oficiais por loja/catálogo**: **Steam (Steamworks)**, **Epic Games Store**, **Apple App Store (iOS/macOS)**, **Google Play Store**, **Sony PlayStation Store (TRC)**, **Xbox / Microsoft Store (XR)**, **Nintendo eShop (Lotcheck)**, **IGDB / Twitch Directory**, **GOG / Itch.io**, **Tabletop / Gráfica, Varejo & BGG** e plataformas customizadas.
- **Checklists Técnicos e Burocráticos/Legais**: itens técnicos (SteamPipe, Steam Deck, EOS, `PrivacyInfo.xcprivacy`/ATT, Android `.aab` 16 KB/Data Safety, TRC, XR, Lotcheck, pré-impressão CMYK) e burocráticos/legais (ClassInd, IARC, ESRB, PEGI, formulários fiscais `W-8BEN-E`, EULA, LGPD/GDPR, direitos de fontes/músicas e divulgação de IA).
- **Assets obrigatórios de Storefront**: slots com dimensões e formatos oficiais de cada loja (ex.: Steam Header Capsule `920×430`, Main `1232×706`, Vertical `748×896`, Library Hero `3840×1240`; Epic `2560×1440` e `1200×1600`; Apple `1024×1024` e iPhone `6.9"` `1320×2868`; Google Play `512×512` e Feature Graphic `1024×500`; Xbox `2160×2160` e `1440×2160`; Switch `1024×1024` e `1920×1080`; IGDB Cover `3:4` `1080×1440`), permitindo upload privado direto com SHA-256, vínculo a versões de artefatos do estúdio e URI `geppetto://` para o arquivo fonte editável (`.kra`/`.psd`).
- **Trava de publicação de Entregas**: quando uma plataforma é vinculada a uma **Entrega**, `Publicar entrega` exige que todos os checklists e assets obrigatórios da loja estejam concluídos (com arquivo/versão anexado) ou marcados como **N/A** mediante justificativa registrada.

## Créditos e entregas

Crie a entrega e selecione os participantes creditados. Revise nomes, funções, seções e ordem antes da aprovação. Uma revisão editorial posterior cria outro rascunho e preserva a versão aprovada anterior.

Registre cada build ou protótipo com o snapshot aprovado de créditos. A publicação da entrega exige critérios, evidências de validação aprovadas e aprovação das versões de artefatos selecionadas. A entrega publicada guarda os IDs das builds e revisões utilizadas.

## Servidor Go dedicado (`studio-server`) e hub entre ferramentas

Além do modo embutido no Wiki.js, o repositório inclui em `studio-server/` um **servidor HTTP em Go independente** que atua como ponto central de integração de projetos entre a Intranet (`/p/studio`), **Tellurian Authentik** (`https://auth.telluriancorp.info`), **Aetherion** (Project Manager da engine), **Geppetto** (editor de arte 2D baseado no Krita), **Lore VCS** e demais ferramentas do estúdio:

- **Persistência unificada no PostgreSQL**: compartilha as tabelas `"studioProjects"` e `"studioFiles"` (adicionando `"studioServiceKeys"` para chaves de serviço `stk_...`) e lê `"users"`, `"groups"` e `"userGroups"` do banco quando implantado junto ao Wiki.js no projeto `roguefairy` da Railway (`rootDirectory: studio-server`).
- **Delegação transparente em `/p/studio`**: quando `STUDIO_API_URL` e `STUDIO_INTERNAL_KEY` estão definidos no serviço `intranet`, os resolvers GraphQL e o controlador de arquivos `/p/studio/files/:projectId` delegam automaticamente leitura, mutações otimistas (`expectedRevision`), exportação de créditos e uploads/downloads binários ao `studio-server` (mantendo a vinculação de páginas Markdown e permissões da wiki localmente no Wiki.js). Sem `STUDIO_API_URL`, o Wiki.js opera normalmente em modo local.
- **Autenticação Multi-App OIDC no Tellurian Authentik**: valida tokens JWT emitidos pelos providers dedicados das ferramentas (`aetherion`, `geppetto`, `rfs-intranet`, `rfs-lore-cli`, `rfs-lore-ci`, `goethe`, `animafaber`, `fontpit`, `carnifex`, `rfs-studio`), inferindo automaticamente qual ferramenta está consultando (`clientApp`) e preservando o isolamento *fail-closed* de `Rogue Fairy Studios - Partners`. Opcionalmente, `STUDIO_AUTHENTIK_API_TOKEN` permite resolver usuários por UUID, username ou e-mail diretamente na API Core v3 do Authentik mesmo antes do primeiro login na wiki.
- **Consultas transversais por usuário e ferramenta**:
  - `GET /v1/me/projects` e `GET /v1/users/{identifier}/projects` — responde *"Usuário X está em quais projetos?"* (por exemplo, quando `client=aetherion`, lista os projetos digitais/híbridos no Project Manager do Aetherion com marcos, tarefas atribuídas, namespace Lore e repositórios Git/Lore prontos para clonar ou abrir).
  - `GET /v1/me/workspaces` e `GET /v1/users/{identifier}/workspaces?vcs=lore` — responde *"Em qual projeto e em quais repositórios do Lore o Usuário Y deve trabalhar para adicionar arte?"* (por exemplo, quando `client=geppetto`, retorna os projetos atribuídos, repositórios `lores://lore.roguefairy.net/<project>/<repo>` com permissão de escrita, locks ativos em `.kra`, artefatos `art_2d` e slots pendentes de cápsulas das lojas).
  - `GET /v1/configs/{tool}` e `GET /v1/projects/{projectRef}/configs/{tool}` — gera manifestos **JSON** e **YAML** (`rfs.studio.config/v1`) para configurar qualquer ferramenta do estúdio de forma generalista (ex.: Authentik + Studio Server no Aetherion, ou Authentik + Lore no Geppetto) ou específica por projeto (ex.: projeto *"Klaus Syndicate"* no Aetherion para desenvolvedores ou no Geppetto para artistas e animadores com repositórios, namespace Lore e níveis de acesso).

## Conectores

O cliente comum publica artefatos e builds, envia resultados de validação, autentica via Device Code no Authentik e consulta projetos/workspaces por usuário. Os adaptadores disponíveis atendem **Aetherion** (aba nativa **Studio** no Project Manager + plugin de editor), **Geppetto/Krita** (janela *Studio Projects & Lore Art...* + publicação 1-clique), Godot e Unity. Goethe, Carnifex e outras engines utilizam os comandos do cliente em seus processos de exportação e build. Consulte `docs/integrations/tool-connectors.md` e `studio-server/README.md` para instalação e contratos.

## Verificação e operação

Execute `npm run test:studio`, `cd studio-server && go test ./...`, `python -m unittest discover -s integrations/tests -v` e `npm run build` para verificar o módulo e a interoperabilidade ao vivo entre Node.js, Go e Python. As migrações `2.5.316` e `2.5.317` são aplicadas pelo Wiki.js, e `studio-server/internal/store/migrations/0001_studio_init.sql` garante idempotentemente o esquema quando o `studio-server` inicializa de forma autônoma.

Os comandos de gestão usam comparação de revisão para evitar sobrescrever alterações simultâneas. Eventos de integração têm IDs estáveis e podem ser reenviados. Arquivos enviados ficam no banco, com acesso condicionado ao projeto e deduplicação por checksum. Referências externas HTTPS permanecem sob as regras do armazenamento externo.

Nesta versão, os registros de gestão são persistidos como um agregado por projeto. Isso mantém cada comando e histórico atômicos, com concorrência controlada no nível do projeto. Os arquivos binários ficam em tabela própria.
