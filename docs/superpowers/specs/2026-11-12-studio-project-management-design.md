# Gestão de produção da Rogue Fairy Studios

Estado: desenho aprovado pelo usuário. Requisito adicional confirmado: gerir participantes das equipes e gerar créditos automaticamente para builds e outras entregas. Os detalhes de implementação desse requisito abaixo são propostas técnicas.

## Objetivo

Transformar a instância Wiki.js do repositório intranet no centro de gestão de jogos digitais e analógicos. Conectar documentação, planejamento, criação, validação e entregas, com integrações para Geppetto, Goethe, Godot, Unity, Carnifex e ferramentas futuras.

## Decisões e abordagens

O usuário confirmou que projetos, tarefas, ciclos e aprovações serão gerenciados na intranet. GitHub e ferramentas de criação fornecerão artefatos e evidências associados a esses registros.

Foram consideradas gestão externa agregada pela wiki e gestão própria na intranet. A segunda foi escolhida. Para implementá-la, propõe-se estender a aplicação existente com módulos de produção e tabelas próprias. Um serviço independente aumentaria os pontos de operação e sincronização. A possibilidade de extração futura pode ser preservada por contratos de integração versionados.

## Base examinada

- package.json identifica Wiki.js 2.5.315.
- server/models contém os modelos de páginas, histórico, usuários, grupos e assets, usando Objection.
- server/graph/index.js carrega schemas e resolvers GraphQL por arquivos.
- server/db contém migrações, incluindo um caminho específico para SQLite.
- client/components contém as telas existentes.

O banco e a configuração efetivos da instância implantada ainda precisam ser verificados. A documentação das ferramentas descreve formatos e integrações, mas sua operação ponta a ponta com a intranet ainda precisa ser implementada e validada.

## Experiência proposta

Na navegação principal, Projetos leva ao portfólio. Cada projeto oferece Visão geral, Documentos, Trabalho, Produção, Validação e Entregas. A visão geral reúne marco atual, responsáveis, bloqueios, revisões pendentes e últimas entregas.

Documentos abrem páginas da wiki vinculadas ao projeto. Trabalho oferece lista e quadro com os mesmos registros, filtros por responsável, disciplina, marco e ciclo. Produção mostra artefatos e versões. Validação reúne testes e playtests. Entregas reúne versões distribuíveis e sua evidência de aprovação.

## Registros e relações

| Registro | Conteúdo principal |
|---|---|
| Projeto | Identidade, tipo digital/analógico/híbrido, equipe, estado e referências |
| Marco | Objetivo, prazo opcional, critérios e entregas |
| Ciclo | Objetivo, período, capacidade planejada e itens associados |
| Item de trabalho | Tipo, disciplina, prioridade, responsável, estado, critérios e dependências |
| Documento vinculado | Página Wiki.js, função documental e revisão de referência |
| Artefato | Identidade estável, disciplina, ferramenta de origem e localização |
| Versão de artefato | Revisão imutável, arquivo ou referência, checksum e origem |
| Revisão | Versão avaliada, revisor, decisão, comentários e data |
| Validação | Caso ou roteiro, execução, versão avaliada, resultado e evidências |
| Entrega | Conjunto versionado de artefatos, critérios e aprovações |

IDs estáveis sustentam os vínculos. Páginas mantêm seus IDs nativos. Uma relação pode apontar para a revisão específica usada em uma decisão. Alterações posteriores indicam necessidade de revisão dos vínculos afetados, preservando o registro histórico da aprovação anterior.

## Documentos ao longo do ciclo

Modelos iniciais incluem visão do projeto, GDD, bíblia narrativa, direção de arte, especificação técnica, histórias e requisitos, decisões de arquitetura, plano de testes, roteiro de playtest, relatório de balanceamento, retrospectiva e notas de entrega.

Cada projeto escolhe os modelos necessários. Documentos existentes podem ser vinculados por seleção explícita. Modelos possuem versão e sua atualização exige aplicação deliberada às páginas já criadas.

Um requisito ou mecânica pode ser ligado a trabalho, artefatos e validação. O GDD pode ser organizado em páginas vinculadas para manter referências estáveis a seções importantes.

## Jogos digitais e analógicos

Projetos digitais vinculam cenas, código, diálogos, assets, builds e plataformas de destino. Projetos analógicos vinculam regras, cartas, fichas, tabuleiros, lista de componentes, protótipos imprimíveis e arquivos de fabricação.

Playtests registram a versão utilizada, roteiro, participantes quando pertinente, observações, resultados e itens de trabalho derivados. A entrega física identifica revisão das regras, componentes e especificações de impressão aprovados.

## Fluxos de trabalho

### Equipes, participação e créditos

Cada projeto mantém equipes, participantes, funções e períodos de participação. Uma pessoa pode exercer várias funções e participar de vários projetos. O cadastro de participante possui identidade própria e vínculo opcional com um usuário da intranet, permitindo incluir colaboradores externos sem conta. Papéis de acesso e funções criativas são campos independentes.

Registrar nome público preferido ou pseudônimo, função de crédito, seção, ordem editorial e contribuições vinculadas a trabalho ou artefatos. Encerrar participação ou desativar acesso preserva o histórico de contribuições e a elegibilidade para créditos. Evitar inclusão automática de toda conta com acesso ao projeto. A geração parte de participações e contribuições explicitamente marcadas para crédito, incluindo registros manuais justificados.

Cada entrega gera um manifesto de créditos versionado a partir dos registros elegíveis, com seções, funções e nomes públicos. A revisão editorial permite corrigir nomes, ordenação, agradecimentos e atribuições de parceiros. A versão aprovada é congelada e associada à entrega. Correções posteriores geram nova revisão, mantendo o manifesto usado em builds anteriores consultável.

Proposta de formatos iniciais: JSON UTF-8 para integração com engines e Markdown para revisão e documentação. O manifesto contém apenas dados públicos de crédito, sem e-mails ou dados internos. A mesma pessoa aparece uma vez por função e seção, preservando créditos em funções distintas. Ordenação usa posição explícita e ID estável como desempate.

Builds consomem o ID e checksum do manifesto aprovado para a entrega. A repetição da geração com as mesmas revisões de entrada produz o mesmo conteúdo. Uma mudança na equipe após a aprovação não altera créditos de uma build já gerada. Protótipos e entregas analógicas também podem usar o manifesto em seus materiais.

A primeira entrega inclui cadastro de participantes e funções. O manifesto e sua exportação entram junto ao registro mínimo de entregas, antes da conexão com as engines.

Fluxo inicial proposto para itens: backlog, pronto, em andamento, em revisão e concluído. Bloqueio é registrado com motivo e dependência. O projeto pode configurar estados e critérios de transição.

Revisões avaliam versões específicas. Novas versões solicitam nova avaliação. Aprovação pode exigir papéis definidos pelo projeto. Conclusão de trabalho, aprovação de um artefato e publicação de uma entrega são ações distintas com registros próprios.

## Integrações

| Ferramenta | Contrato proposto |
|---|---|
| Geppetto | Consultar briefing e trabalho, vincular fonte editável, enviar preview/exportação e registrar versão |
| Goethe | Vincular personagens e diálogos, registrar revisão de fonte e pacote, comunicar validação narrativa e localização |
| Godot e Unity | Associar recursos e cenas, consultar trabalho, registrar builds e resultados de testes |
| Carnifex e outras engines | Utilizar contrato comum por adaptador de editor ou linha de comando |
| GitHub | Relacionar commits, pull requests e resultados de automação aos itens e entregas |

A primeira etapa de cada conector deve verificar as capacidades reais da ferramenta. Links de arquivo e importações manuais podem apoiar o uso inicial. Cada integração completa exige teste com a ferramenta de origem e a intranet.

Eventos de integração incluem projeto, ID externo, versão do contrato e chave de idempotência. Processamento registra tentativas, falhas e resultado, permite reenvio e reconciliação. Eventos atrasados preservam a ordem das versões. Atualizações concorrentes exigem comparação de revisão e resolução explícita de conflitos.

Arquivos grandes permanecem em armazenamento de artefatos adequado, referenciado pela intranet. A seleção desse armazenamento depende da infraestrutura existente. Downloads e previews respeitam o acesso ao projeto.

## Acesso e consistência

Reutilizar identidade e grupos existentes, com autorização por projeto e ação. Papéis iniciais propostos: administrador de projeto, produtor, colaborador, revisor e leitor. Parceiros recebem escopo explícito por projeto e conteúdo.

Todas as operações, pesquisas, vínculos e downloads verificam autorização no servidor. Aprovações e alterações relevantes registram autoria e horário. As integrações usam identidade de serviço com escopo limitado, definida na etapa de implantação.

Transições e criação de vínculos devem ser transacionais. Referências externas indisponíveis permanecem identificadas. Arquivamento preserva histórico e relações. Migrações devem ser testadas com o banco efetivo e restauração verificada antes da implantação.

## Entregas incrementais

1. Fundação utilizável: projetos, membros, vínculos com páginas existentes, itens de trabalho, lista/quadro e histórico. Validar criação de projeto e acompanhamento de uma mecânica do GDD até a conclusão de uma tarefa.
2. Planejamento e revisão: marcos, ciclos, dependências, critérios, versões e aprovações.
3. Produção integrada: contrato comum e conectores Geppetto e Goethe, depois Godot, Unity e Carnifex. Cada conector tem validação independente.
4. Validação e entregas: testes, playtests, builds, protótipos físicos, releases e indicadores derivados dos registros.

O projeto piloto será escolhido com o usuário. Sugestão inicial: Vox Spectra para o fluxo digital e um pequeno projeto analógico para validar regras, componentes e playtests.

## Critérios de sucesso

- Um projeto gerencia trabalho, responsáveis, ciclos e aprovações inteiramente na intranet.
- É possível navegar do design até uma entrega e identificar as revisões de artefatos e validações utilizadas.
- O fluxo funciona para jogos digitais e analógicos.
- Um evento repetido de integração gera um único efeito e uma falha pode ser recuperada.
- Uma nova versão mantém decisões anteriores consultáveis e identifica revisões pendentes.
- Acesso de parceiros respeita o escopo autorizado em telas, API, pesquisa e arquivos.
- Páginas existentes continuam acessíveis e podem ser incorporadas aos projetos.

## Próximo detalhamento

### Decisões incorporadas após revisão

- Acesso a uma página vinculada exige autorização tanto no projeto quanto na página Wiki.js. Uma página compartilhada por projetos mantém suas permissões próprias. Listagens exibem somente vínculos autorizados.
- Na primeira entrega, a referência documental identifica página e revisão. Mecânicas ou requisitos que precisem de identidade própria usam páginas dedicadas. Âncoras de títulos servem para navegação.
- Antes dos conectores da etapa 3, criar os contratos e registros mínimos de validação e entrega necessários ao recebimento de builds e resultados. A etapa 4 completa as telas e os fluxos de gestão desses registros.

Após revisão do desenho, preparar plano de implementação da primeira entrega com migrações, autorização, navegação, contratos e verificações. Antes de implementar, confirmar banco efetivo, mecanismo de sessão e pontos de autorização no código, política de atualização do fork e critérios do projeto piloto.
