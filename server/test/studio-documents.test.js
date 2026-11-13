const studio = require('../helpers/studio')
const templates = require('../helpers/studio-templates')
const api = require('../graph/resolvers/studio')

describe('studio document templates', () => {
  test('covers digital, analogue and production documents in both languages', () => {
    expect(templates.list('pt-br').map(item => item.key)).toEqual(templates.list('en').map(item => item.key))
    expect(templates.render('tabletop', 'pt-br', 'Regras')).toContain('Lista de componentes')
    expect(templates.render('gdd', 'en', 'Design')).toContain('Gameplay loop')
    expect(templates.render('pipeline', 'pt-br', 'Pipeline')).toContain('Lore')
    expect(templates.render('pipeline', 'en', 'Pipeline')).toContain('goethe://')
    expect(templates.render('storefront', 'pt-br', 'Lojas')).toContain('ClassInd')
    expect(templates.render('storefront', 'en', 'Storefronts')).toContain('Lotcheck')
    expect(() => templates.render('unknown', 'en', 'Title')).toThrow()
  })
  test('reuses a created draft after a concurrent project update', async () => {
    let page = null
    let conflict = true
    const state = studio.createState('Game', 'digital', 8)
    const record = { id: state.projectId, revision: 1, state: JSON.stringify(state) }
    const createPage = jest.fn(async options => {
      page = { id: 5, path: options.path, localeCode: options.locale, title: options.title, isPublished: false, updatedAt: 'revision-1', tags: options.tags.map(tag => ({ tag })) }
      return page
    })
    global.WIKI = {
      data: { reservedPaths: [] },
      config: { lang: { code: 'en' } },
      auth: { checkAccess: () => true },
      models: {
        users: { query: () => ({ findById: async id => ({ id, isActive: true }) }) },
        pages: { createPage, query: () => ({ findOne: () => ({ withGraphFetched: async () => page }), findById: () => ({ withGraphFetched: async () => page }) }) },
        studioProjects: { query: () => ({ findById: async () => ({ ...record }),
          where: () => ({ patch: async patch => {
            if (conflict) { conflict = false; record.revision++; return 0 }
            Object.assign(record, patch)
            return 1
          } }) }) }
      }
    }
    const args = { projectId: record.id, revision: 1, command: 'createDocument', input: JSON.stringify({ template: 'gdd', title: 'Game design', path: 'projects/game/gdd', locale: 'en' }) }
    const context = { req: { user: { id: 8 } } }
    try {
      await expect(api.Mutation.studioCommand(null, args, context)).rejects.toThrow('Reload')
      args.revision = record.revision
      await api.Mutation.studioCommand(null, args, context)
      expect(createPage).toHaveBeenCalledTimes(1)
      expect(JSON.parse(record.state).documents[0].template).toBe('gdd')
    } finally { delete global.WIKI }
  })
})
