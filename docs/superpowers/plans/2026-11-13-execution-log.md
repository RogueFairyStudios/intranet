# Registro da implementação

Branch `feat/studio-management`, cópia em `D:/roguefairy/intranet-studio`.

## Decisões

- Execução local autorizada pelo pedido para continuar. A ferramenta de worktree gerenciada rejeitou os argumentos e a criação foi realizada com Git.
- A primeira implementação usa um agregado JSON por projeto, persistido pelo modelo Objection `studioProjects`, com atualização condicional por revisão. Cada comando e seu histórico são persistidos atomicamente. Esta é uma alteração ao desenho de tabelas individuais, escolhida para a fundação inicial. Limite conhecido: concorrência e consultas são por projeto inteiro. Normalização será necessária antes de ampliar escala e relatórios transversais.
- A tela foi consolidada em `client/components/studio/projects.vue` com tradução em `messages.js`, reaproveitando shell, sessão e cliente Apollo. A rota `/p/studio` usa o namespace de perfil já reservado.
- Na fundação, administradores criam projetos e gestores concedem acesso por ID de usuário. Participantes externos não precisam de conta. Créditos são selecionados explicitamente por entrega.
- O piloto usa dados sintéticos digitais e analógicos. Nenhum projeto real foi importado automaticamente.

## Entregue no código

Projetos, participantes e funções múltiplas, período de participação, acesso por projeto, vínculos de páginas, tarefas com critérios e responsáveis, lista/quadro, registro mínimo de entregas, geração/revisão/aprovação/exportação de créditos, associação de créditos a builds e histórico de comandos.

## Verificação

- 23 testes do servidor passaram, incluindo autorização, conflito de revisão, créditos imutáveis e correções editoriais.
- Compilação de produção concluída, com avisos de tamanho de bundles existentes.
- Lint direcionado aos arquivos novos passou.
- Revisão independente encontrou exposição de títulos não publicados e ausência de revisão editorial após aprovação. Ambos corrigidos com testes de regressão.
- Fluxos API/modelo/banco passaram em emulação PostgreSQL para jogos digitais e analógicos, incluindo escritas concorrentes e preservação dos créditos.
- Rollback removeu a tabela na emulação. Reaplicação esbarrou em índice residual do emulador. Validação de migração e restauração em banco real continua pendente.
- Verificação geral de lint encontrou 248 erros e 5 avisos no repositório. A execução geral do Jest também coleta testes Cypress e falha por ausência de `cy`. Os testes em `server/test` passaram.

## Pendências de validação e continuidade

Verificação visual no navegador, validação na instância implantada, migração em banco real, interfaces de seleção de usuários/páginas por pesquisa, filtros avançados, ciclos, marcos, relações entre artefatos e conectores de ferramentas. O cadastro de equipes é inicialmente o agrupamento nominal dos participantes. A seleção de contribuições para créditos é inicialmente por participante e função.


## Fechamento ampliado em 13/11/2026

O pedido para continuar até o fim ampliou a implementação para planejamento, produção, validação e conectores do desenho aprovado.

- Equipes possuem registros próprios dentro do projeto, com arquivamento. Participantes têm conta opcional e múltiplas funções.
- Marcos, ciclos, tarefas editáveis, filtros, prioridades, dependências e bloqueios estão implementados.
- Documentos nativos podem ser criados a partir de oito modelos versionados. Vínculos detectam mudanças de revisão. Criação interrompida por conflito pode ser repetida sem duplicar a página.
- Artefatos, versões imutáveis, revisões, testes, playtests e publicação de entregas estão implementados.
- Eventos idempotentes, identidades de serviço por grupo e auditoria distinta de usuários humanos foram incluídos.
- Arquivos privados ficam em tabela própria no banco, com autorização por projeto e deduplicação por hash.
- Cliente Python com fila durável, reenvio e conferência de checksum foi exercitado contra a API HTTP e PostgreSQL reais.
- Pacotes de adaptadores foram escritos para Geppetto/Krita, Godot e Unity. Goethe e Carnifex usam o cliente comum em seus processos de exportação e build.
- PostgreSQL 16 passou nos fluxos digitais e analógicos, concorrência, migração, rollback e reaplicação. Backup da base de teste foi restaurado em outro banco com os projetos e arquivos preservados.
- O navegador executou criação de equipe, participante externo, marco, ciclo, tarefa e créditos. Um fluxo analógico separado chegou à publicação de uma entrega com playtest e créditos aprovados. A tela de 390 pixels não apresentou rolagem horizontal nesse fluxo.
- Godot 4.3 carregou o plugin real em modo editor/headless. Foi corrigido um conflito com o método virtual _build. O adaptador Unity compilou contra substitutos da API do editor para conferência de sintaxe. O Python do Geppetto passou na conferência de sintaxe.
- A revisão independente identificou conflitos de conteúdo de build, isolamento de erros na fila e autoria de eventos de API. Os problemas foram corrigidos com regressões automatizadas.

### Limites de implantação

O código foi validado localmente. A instância de produção não foi modificada. Os adaptadores ainda precisam ser ativados nos projetos reais e, para Unity e Geppetto, exercitados nos respectivos editores instalados. O agrupamento persistente por projeto permanece como decisão técnica desta versão, registrada acima.

## Auditoria final, correções de integração PostgreSQL/SSO e alinhamento visual (13/11/2026)

- **Correções críticas de runtime e PostgreSQL:**
  - Corrigida a query GraphQL corrompida em `client/components/studio/search-picker.vue` (`$projectId: String!`) e adicionado carregamento inicial automático de opções ao abrir seletores de usuários, grupos e páginas.
  - Substituído `LIKE` sensível a maiúsculas/minúsculas no PostgreSQL em `server/graph/resolvers/studio.js` por `LOWER(??) LIKE ?` (compatível com PostgreSQL e SQLite), estendendo `studioPages` para buscar tanto por `title` quanto por `path`, além de enriquecer nomes de usuários (`access`) e grupos (`integrationAccess`) na visão de gestores e ocultá-los para não-gestores.
- **Integração com SSO, i18n e tema monocromático da Intranet:**
  - `server/views/studio.pug` e `server/controllers/common.js` passaram a injetar `injectCode` (`css`, `head`, `body`), `pageMeta.title`, suporte a idioma (`?lang=pt-br` / `?lang=en`) e redirecionamento automático `loginRedirect` -> `/login` (Authentik OIDC) para visitantes não autenticados.
  - `client/components/common/nav-header.vue` ajustado para exibir o botão `Projetos`/`Projects` apenas a usuários autenticados, preservar o idioma ativo da intranet ao clicar no logotipo e habilitar o seletor de idiomas em `/p/studio`.
  - Paleta roxa legada removida de `client/components/studio/projects.vue` e substituída pelo sistema visual sóbrio monocromático escuro/claro da Rogue Fairy Intranet (`#0d0d0f`, `#141417`, `#18181c`, `#26262a`, `#f4f4f5`, `Inter`).
- **Fechamento de escopo de UX:**
  - Adicionada a aba **Visão geral** (`overview`) conforme a especificação de design (marco atual, ciclo ativo, tarefas abertas/concluídas, participantes ativos, bloqueios, revisões pendentes, documentos desatualizados e últimas entregas) e sincronização de deep-links (`?project=...&tab=...`).
  - Adicionado componente `client/components/studio/file-upload.vue` integrado a versões de artefatos e registro de builds para upload direto pelo navegador em `POST /p/studio/files/:projectId` com cálculo automático de SHA-256.
  - Formulário de criação de projeto restrito na UI a administradores (`manage:system`).
