# Gestão de produção na intranet

Abra Projetos no cabeçalho da intranet ou acesse `/p/studio`. Visitantes não autenticados são redirecionados automaticamente para o SSO (Tellurian Authentik) e retornam ao destino após o login. Um administrador cria o projeto como digital, analógico ou híbrido e concede acesso aos gestores e colaboradores.

A seção inicial **Visão geral** consolida marco atual, ciclo ativo, tarefas abertas/concluídas, participantes ativos, progresso de integração de assets na engine, saúde dos repositórios VCS (Git e Lore/Epic URC), bloqueios em aberto, revisões pendentes, documentos desatualizados e últimas entregas, com links profundos preservados na URL (`?project=<id>&tab=<secao>&lang=<idioma>`).

## Pipeline de desenvolvimento, URIs de ferramentas e VCS (Git & Lore)

A seção **Pipeline** reflete as duas camadas das pipelines modernas de jogos:

1. **Fases macro do projeto (`pre_production`, `production`, `post_production`, `live`)**: acompanha em que estágio macro o projeto se encontra, quantos marcos e tarefas pertencem a cada fase e permite aos gestores avançar ou ajustar a fase ativa do projeto.
2. **Matriz não-linear de pipeline de assets (`concept`, `blockout`, `authoring`, `surface_loc`, `engine_integration`, `polish`)**: cada artefato/asset é classificado por disciplina (*track*), ferramenta do estúdio (`Goethe`, `Geppetto`, `Godot`, `Carnifex`, `AnimaFaber`, `Fontpit`, `Lore`, `UEFN`, etc.), caminho no repositório e URI direta de lançamento (`goethe://`, `geppetto://`, `godot://`, `carnifex://`, `lore://`, `uefn://`, `animafaber://`, `fontpit://`). Qualquer célula da matriz pode ser acionada para avançar **ou retornar** um asset de forma não-linear após testes na engine ou revisão de direção.
3. **Versionamento Git & Lore (Epic Unreal Revision Control)**: registra os repositórios vinculados ao projeto (`git`, `lore`, `urc`), exibindo branch/stream atual, revisão (`SHA` ou `branch@N` BLAKE3), estado da working tree (limpa ou com modificações locais) e **locks exclusivos de arquivos binários** do Lore/URC correlacionados diretamente nas linhas da matriz de assets.

## Equipe e acesso

Cadastre equipes e participantes. Pessoas externas podem ser incluídas sem conta de acesso. Cada participante possui nome público, funções nos créditos, equipe e período de participação. O vínculo opcional com uma conta facilita identificar membros internos. O acesso à gestão é configurado separadamente como leitor, colaborador ou gestor.

Encerrar uma participação preserva tarefas históricas e créditos. Arquivar uma equipe preserva seus membros históricos. Arquivar o projeto suspende alterações de produção até a reativação por um gestor.

## Documentação e planejamento

Em Documentos, escolha um modelo, título, caminho e idioma para gerar um rascunho no editor Markdown nativo da wiki. Os modelos têm versão identificada. Também é possível buscar e vincular páginas existentes. As permissões da página continuam sendo aplicadas. Quando a página muda, a referência mostra a necessidade de revisão e pode ser atualizada deliberadamente.

Cadastre marcos com critérios de aceite e ciclos com objetivos, períodos e capacidade. As tarefas podem indicar responsável, disciplina, prioridade, prazo, bloqueio, dependências, marco, ciclo e documentos relacionados. Lista e quadro mostram os mesmos registros. Dependências circulares são rejeitadas. A conclusão exige resolver bloqueios e dependências abertas.

## Produção e validação

Cadastre artefatos associados às tarefas e documentos. Cada versão possui localização e SHA-256 e permanece imutável — na interface web, você pode enviar o arquivo diretamente para o armazenamento privado do projeto (`/p/studio/files/:projectId`), preenchendo URL e SHA-256 automaticamente, ou informar uma URI externa. Gestores registram revisão e aprovação de versões específicas. O histórico conserva as decisões anteriores.

Registre testes e playtests com resultado, observações e referências à tarefa, entrega ou versão avaliada. Jogos analógicos podem usar artefatos de cartas, regras, tabuleiros, componentes e arquivos de impressão no mesmo fluxo.

## Créditos e entregas

Crie a entrega e selecione os participantes creditados. Revise nomes, funções, seções e ordem antes da aprovação. Uma revisão editorial posterior cria outro rascunho e preserva a versão aprovada anterior.

Registre cada build ou protótipo com o snapshot aprovado de créditos. A publicação da entrega exige critérios, evidências de validação aprovadas e aprovação das versões de artefatos selecionadas. A entrega publicada guarda os IDs das builds e revisões utilizadas.

## Conectores

O cliente comum publica artefatos e builds, envia resultados de validação e importa créditos aprovados. Os adaptadores disponíveis atendem Geppetto/Krita, Godot e Unity. Goethe, Carnifex e outras engines utilizam os comandos do cliente em seus processos de exportação e build. Consulte `docs/integrations/tool-connectors.md` para instalação e contratos.

## Verificação e operação

Execute `npm run test:studio`, `python -m unittest discover -s integrations/tests -v` e `npm run build` para verificar o módulo. As migrações `2.5.316` e `2.5.317` são aplicadas pelo mecanismo existente do Wiki.js. A primeira cria a gestão por projeto e a segunda cria os arquivos privados.

Os comandos de gestão usam comparação de revisão para evitar sobrescrever alterações simultâneas. Eventos de integração têm IDs estáveis e podem ser reenviados. Arquivos enviados ficam no banco, com acesso condicionado ao projeto e deduplicação por checksum. Referências externas HTTPS permanecem sob as regras do armazenamento externo.

Nesta versão, os registros de gestão são persistidos como um agregado por projeto. Isso mantém cada comando e histórico atômicos, com concorrência controlada no nível do projeto. Os arquivos binários ficam em tabela própria.
