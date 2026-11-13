const templates = {
  gdd: {
    pt: ['Game Design Document', 'Visão e pilares', 'Público e plataformas', 'Loop de jogo', 'Regras e mecânicas', 'Narrativa e personagens', 'Arte e áudio', 'Interface e acessibilidade', 'Escopo, riscos e decisões', 'Critérios de validação'],
    en: ['Game Design Document', 'Vision and pillars', 'Audience and platforms', 'Gameplay loop', 'Rules and mechanics', 'Narrative and characters', 'Art and audio', 'Interface and accessibility', 'Scope, risks and decisions', 'Validation criteria']
  },
  technical: {
    pt: ['Especificação técnica', 'Objetivo', 'Requisitos e restrições', 'Arquitetura e interfaces', 'Dados e persistência', 'Falhas e recuperação', 'Segurança e permissões', 'Testes e critérios de aceite', 'Decisões pendentes'],
    en: ['Technical specification', 'Objective', 'Requirements and constraints', 'Architecture and interfaces', 'Data and persistence', 'Failures and recovery', 'Security and permissions', 'Tests and acceptance criteria', 'Open decisions']
  },
  art: {
    pt: ['Direção de arte', 'Intenção visual', 'Referências e linguagem', 'Personagens e cenários', 'Paleta, tipografia e composição', 'Formatos e exportações', 'Revisão e aprovação'],
    en: ['Art direction', 'Visual intent', 'References and visual language', 'Characters and environments', 'Palette, typography and composition', 'Formats and exports', 'Review and approval']
  },
  narrative: {
    pt: ['Bíblia narrativa', 'Premissa e temas', 'Mundo e cronologia', 'Personagens e relações', 'Arcos e escolhas', 'Diálogos e localização', 'Cânone e decisões pendentes'],
    en: ['Narrative bible', 'Premise and themes', 'World and chronology', 'Characters and relationships', 'Arcs and choices', 'Dialogue and localisation', 'Canon and open decisions']
  },
  playtest: {
    pt: ['Plano de playtest', 'Hipótese', 'Versão e componentes', 'Participantes', 'Roteiro e observações', 'Resultados', 'Balanceamento', 'Ações e próximos testes'],
    en: ['Playtest plan', 'Hypothesis', 'Version and components', 'Participants', 'Protocol and observations', 'Results', 'Balancing', 'Actions and next tests']
  },
  test: {
    pt: ['Plano de testes', 'Escopo e requisitos', 'Ambientes e versões', 'Casos e resultados esperados', 'Execução e evidências', 'Defeitos encontrados', 'Critérios de saída'],
    en: ['Test plan', 'Scope and requirements', 'Environments and versions', 'Cases and expected results', 'Execution and evidence', 'Defects found', 'Exit criteria']
  },
  retrospective: {
    pt: ['Retrospectiva', 'Objetivo e período', 'Resultados', 'O que funcionou', 'Dificuldades', 'Ações, responsáveis e prazos'],
    en: ['Retrospective', 'Objective and period', 'Results', 'What worked', 'Difficulties', 'Actions, owners and deadlines']
  },
  tabletop: {
    pt: ['Regras e componentes', 'Objetivo do jogo', 'Jogadores e duração', 'Lista de componentes', 'Preparação', 'Estrutura de turno', 'Condições de vitória e empate', 'Exemplos e casos especiais', 'Impressão e fabricação'],
    en: ['Rules and components', 'Game objective', 'Players and duration', 'Component list', 'Setup', 'Turn structure', 'Victory and tie conditions', 'Examples and edge cases', 'Printing and manufacturing']
  }
}

function list (locale) {
  const language = locale.startsWith('pt') ? 'pt' : 'en'
  return Object.keys(templates).map(key => ({ key, title: templates[key][language][0], version: 1 }))
}

function render (key, locale, title) {
  if (!Object.prototype.hasOwnProperty.call(templates, key)) throw new Error('Unknown document template')
  const sections = templates[key][locale.startsWith('pt') ? 'pt' : 'en']
  return '# ' + title.replace(/[\r\n]/g, ' ') + '\n\n' + sections.slice(1).map(section => '## ' + section + '\n').join('\n')
}

module.exports = { list, render }
