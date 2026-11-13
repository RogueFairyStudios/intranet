# Créditos por entrega

A interface está em `/p/studio`, disponível pelo botão Projetos no cabeçalho. Administradores criam projetos. Gestores concedem acesso a usuários existentes pelo ID. O cadastro de participantes é independente das contas e aceita colaboradores externos.

Na seção Créditos e entregas, crie uma entrega, selecione seus participantes, gere o rascunho e revise as entradas. Um gestor aprova a versão. Downloads JSON e Markdown usam essa versão congelada. Criar revisão duplica créditos aprovados em um novo rascunho para correções editoriais.

## Contrato v1

O manifesto público contém `schemaVersion`, `projectId`, `releaseId`, `title` e `entries`. Cada entrada contém `participantId`, `name`, `role`, `section` e `order`. O checksum SHA-256 é calculado sobre os bytes UTF-8 do JSON compacto retornado pela API. Preservar esses bytes ao verificar o checksum.

```graphql
query Credits($projectId: String!, $snapshotId: String!) {
  studioCredits(projectId: $projectId, snapshotId: $snapshotId, format: "json")
}
```

As operações usam a sessão autenticada da intranet e exigem acesso ao projeto. A integração de identidades de serviço para agentes de build será definida junto aos conectores. A API atual valida contas humanas ativas.

Registrar uma build usa `studioCommand` com `command: "build"` e entrada JSON contendo `name`, `releaseId` e `snapshotId`. A API exige créditos aprovados da mesma entrega. Uma repetição com o mesmo identificador e snapshot preserva a associação. Uma tentativa de reutilizar o identificador com créditos diferentes falha.

Todas as mutações exigem a revisão corrente do projeto. Em conflito, recarregar e reaplicar a intenção do usuário. O snapshot aprovado permanece imutável quando nomes, funções ou participações mudam.

## Verificação local e limites

A migração `2.5.316` cria `studioProjects`, com implementação compartilhada entre os caminhos normal e SQLite. Esta entrega armazena os registros do projeto em um agregado JSON com controle otimista de revisão. Isso mantém cada comando atômico, incluindo seu histórico. Projetos grandes e consultas entre projetos exigirão extração para tabelas e índices próprios antes de expansão de escala.

A interface inclui participantes, funções, encerramento de participação, acesso por projeto, vínculos de páginas, lista/quadro de tarefas, entregas e créditos. Ciclos completos, conectores de editores/engines, upload de artefatos e publicação automática permanecem nas próximas etapas.

As alterações estão em uma cópia isolada de trabalho. A instância implantada e seu banco efetivo ainda precisam de validação antes de publicar a atualização.
