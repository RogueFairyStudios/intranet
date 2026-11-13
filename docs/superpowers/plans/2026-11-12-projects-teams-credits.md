# Projetos, equipes e créditos — plano de implementação

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Objetivo**

Entregar a fundação da gestão na intranet com projetos, participantes, documentos vinculados, tarefas e créditos versionados para entregas.

**Arquitetura**

Estender a aplicação Wiki.js existente com modelos Objection, operações GraphQL e telas integradas. Participantes possuem identidade independente da conta de acesso. Créditos publicados são snapshots imutáveis consumidos pelas ferramentas de build.

**Tecnologias**

Node.js, Objection/Knex, GraphQL e componentes Vue existentes. Confirmar banco implantado antes de escrever migrações.

**Especificação**

`docs/superpowers/specs/2026-11-12-studio-project-management-design.md`

## Restrições gerais

- Gestão de projetos e trabalho acontece na intranet.
- Autorização é validada no servidor em todas as operações.
- Páginas vinculadas exigem acesso à página e ao projeto.
- Função de crédito e papel de acesso são independentes.
- Encerrar participação preserva créditos e histórico.
- Manifestos de créditos incluem somente dados públicos.
- Esta primeira entrega fornece exportação e contrato de consumo. Os conectores de cada engine são entregas posteriores com validação ponta a ponta.

## Pontos de revisão

1. Colaborador externo sem conta deve poder receber crédito.
2. Participante desligado deve permanecer nos créditos aplicáveis.
3. Pessoa com várias funções deve manter todas elas sem duplicação na mesma função/seção.
4. Alterações concorrentes não podem modificar silenciosamente uma revisão aprovada.
5. Usuário sem acesso não pode obter informações por exportações ou vínculos de documentos.

## Tarefa 1 — confirmar os pontos de extensão

- [ ] Inspecionar `server/db/migrator-source.js`, migrações existentes e seleção de banco para definir numeração e compatibilidade da migração.
- [ ] Inspecionar `server/graph/directives`, `server/graph/resolvers/page.js`, `server/models/groups.js` e autorização de páginas para reutilizar verificações existentes.
- [ ] Inspecionar `client/client-app.js`, `client/index-app.js`, `server/controllers` e templates para localizar o registro de uma tela de projetos e sua navegação.
- [ ] Registrar no plano os arquivos concretos de roteamento e migração encontrados antes de editar esses pontos. Confirmar a configuração implantada por metadados, sem ler valores secretos.

Resultado verificável: mapa de entrada da tela, autorização e migração compatível com a instância.

## Tarefa 2 — projetos e participantes

Criar `server/models/studioProjects.js`, `server/models/studioParticipants.js`, `server/models/studioMemberships.js`, `server/helpers/studio-access.js`, `server/graph/schemas/studio.graphql` e `server/graph/resolvers/studio.js`. Adicionar migração no caminho identificado na tarefa 1.

Interfaces propostas: `assertProjectAccess(actor, projectId, action)`, `createProject(input)`, `addParticipant(projectId, input)`, `updateParticipation(id, expectedRevision, input)`.

- [ ] Criar tabelas de projetos, equipes, participantes, participações, funções e histórico com IDs estáveis. Participante possui `userId` opcional, `displayName` e revisão. Participações possuem projeto, equipe opcional, períodos e estado. Funções são registros múltiplos por participação.
- [ ] Implementar operações transacionais e controle de revisão. A autorização deriva da associação de acesso, independente do cadastro de crédito.
- [ ] Criar testes em `server/test/studio-projects.test.js` para criação autorizada, negação de acesso, participante externo, múltiplas funções e encerramento sem exclusão do histórico.
- [ ] Executar testes direcionados e corrigir falhas antes de seguir.

## Tarefa 3 — documentação e trabalho

Criar `server/models/studioWorkItems.js` e `server/models/studioDocumentLinks.js`. Estender schema e resolver studio.

Interfaces propostas: `linkDocument(projectId, pageId, pageRevision)`, `createWorkItem(projectId, input)`, `transitionWorkItem(id, expectedRevision, state)`.

- [ ] Criar vínculos de página/revisão e tarefas com responsável, critérios, disciplina e estados definidos na especificação.
- [ ] Validar participação do responsável e autorização da página em leitura e escrita. Referências antigas preservam identificação quando conteúdo deixa de estar acessível.
- [ ] Registrar mudanças de estado em histórico na mesma transação.
- [ ] Testar em `server/test/studio-work.test.js` acesso combinado, conflito de revisão, pessoa desligada em tarefa histórica e página vinculada a projetos diferentes.

## Tarefa 4 — entregas mínimas e créditos

Criar `server/models/studioReleases.js`, `server/models/studioCreditSnapshots.js` e `server/helpers/studio-credits.js`. Estender schema e resolver studio.

Interfaces propostas: `generateCredits(projectId, releaseId, expectedRevision)`, `approveCredits(snapshotId, expectedRevision)`, `exportCredits(snapshotId, format)`.

Manifesto v1: `schemaVersion`, `projectId`, `releaseId`, `sections[]`. Cada seção possui ID, título, ordem e entradas com participante, nome público, função e ordem. Metadados internos armazenam revisão de origem, autoria e data fora do conteúdo público. Checksum calculado sobre serialização JSON canônica UTF-8.

- [ ] Criar registro mínimo de entrega e seleção explícita das participações/contribuições elegíveis para seus créditos.
- [ ] Implementar geração determinística, deduplicação por participante/função/seção e revisão editorial com motivo de alteração.
- [ ] Congelar snapshot aprovado. Alterações criam novo snapshot e preservam os anteriores.
- [ ] Exportar JSON e Markdown com escape adequado. Validar acesso ao projeto e estado aprovado para consumo por builds.
- [ ] Registrar em cada build ou entrega importada `creditSnapshotId` e checksum. Rejeitar referência a snapshot de outro projeto ou entrega.
- [ ] Testar em `server/test/studio-credits.test.js` os cinco pontos de revisão, ordenação estável, nomes Unicode, ausência de dados privados, correção após aprovação, repetição determinística e checksum.

## Tarefa 5 — experiência dentro da intranet

Criar `client/components/studio/projects.vue`, `project.vue`, `team.vue`, `work.vue` e `credits.vue`, com operações em `client/graph/studio/`. Registrar telas nos pontos confirmados na tarefa 1.

- [ ] Implementar portfólio, criação e detalhes de projeto.
- [ ] Implementar participantes, equipes, funções e encerramento de participação, incluindo pessoas sem conta.
- [ ] Implementar vínculos de documentos e visualizações lista/quadro das mesmas tarefas.
- [ ] Implementar seleção de entrega, preview de créditos, ajustes, aprovação e download.
- [ ] Mostrar conflitos de revisão com opção de recarregar os dados. Tratar estados vazios, falhas e permissões insuficientes.
- [ ] Usar identidade visual existente e textos pt-BR/en-GB. Verificar navegação por teclado e formulários em tela estreita.

## Tarefa 6 — validação integrada

- [ ] Executar `yarn test` e `yarn build`, registrando limitações do ambiente quando existentes.
- [ ] Verificar aplicação e restauração de migração em banco de teste compatível.
- [ ] Exercitar um projeto digital e um analógico desde documento e tarefa até entrega com créditos.
- [ ] Exportar manifesto, alterar nome/função na equipe e confirmar que a versão aprovada anterior permanece idêntica.
- [ ] Testar acesso de parceiro em páginas, API e downloads.
- [ ] Documentar contrato v1 e exemplos públicos em `docs/integrations/credits-v1.md`.

## Continuidade do desenho aprovado

Marcos e ciclos completos, dependências avançadas, gestão geral de versões de artefatos, revisões artísticas, conectores Geppetto/Goethe/engines, testes/playtests completos e publicação serão detalhados em planos seguintes. O contrato de créditos e o registro mínimo de entrega desta etapa são pré-requisitos dos conectores de build.


## Resultado da execução

As capacidades das seis tarefas foram implementadas e verificadas localmente. O registro de execução de 13/11/2026 descreve os caminhos concretos, testes e desvios de estrutura. Foram mantidos modelos agregados por projeto em vez dos modelos individuais propostos, e componentes de formulário, pesquisa, créditos e entregas foram extraídos do componente principal. O contrato público v1 usa entries com section e order.

As etapas posteriores de marcos, ciclos, versões de artefatos, revisões, validações e conectores também foram implementadas no mesmo ramo. A publicação na instância de produção e a ativação dos adaptadores nos projetos reais são etapas de integração operacional.
