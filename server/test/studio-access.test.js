const studio = require('../helpers/studio')
const resolver = require('../graph/resolvers/studio')

describe('studio API authorization and revisions', () => {
  let record
  let pageAllowed
  let pagePublished
  const context = id => ({ req: { user: { id, permissions: [] } } })
  beforeEach(() => {
    pageAllowed = false
    pagePublished = true
    const state = studio.createState('Game', 'digital', 8)
    state.access = [{ userId: 9, role: 'reader' }, { userId: 10, role: 'editor' }]
    record = { id: 'project', revision: 1, state: JSON.stringify(state) }
    global.WIKI = {
      auth: { checkAccess: (user, permissions) => permissions.includes('read:pages') && pageAllowed },
      models: {
        users: { query: () => ({
          findById: async id => ({ id, name: `User ${id}`, isActive: id !== 11 }),
          select () { return this },
          where () { return this },
          whereNot () { return this },
          whereRaw (sql, bindings) { this.raw = { sql, bindings }; return this },
          orderBy () { return this },
          async limit () { return [{ id: 9, name: 'Vox Spectra' }].filter(u => u.name.toLowerCase().includes(this.raw.bindings[1].replace(/%/g, ''))) }
        }) },
        groups: { query: () => ({
          findById: async id => ({ id, name: `Group ${id}` }),
          select () { return this },
          whereRaw (sql, bindings) { this.raw = { sql, bindings }; return this },
          orderBy () { return this },
          async limit () { return [{ id: 4, name: 'Geppetto Pipeline' }].filter(g => g.name.toLowerCase().includes(this.raw.bindings[1].replace(/%/g, ''))) }
        }) },
        studioProjects: { query: () => ({
          findById: async id => id === record.id ? { ...record } : null,
          where: condition => ({ patch: async patch => {
            if (condition.revision !== record.revision) return 0
            Object.assign(record, patch)
            return 1
          } })
        }) },
        pages: { query: () => ({
          findById: () => ({ withGraphFetched: async () => ({ id: 1, title: 'Abyssum Design', path: 'games/abyssum', localeCode: 'en', tags: [], isPublished: pagePublished }) }),
          select () { return this },
          where (fn) {
            const clauses = []
            fn({
              whereRaw (sql, bindings) { clauses.push({ sql, bindings }); return this },
              orWhereRaw (sql, bindings) { clauses.push({ sql, bindings }); return this }
            })
            this.clauses = clauses
            return this
          },
          orderBy () { return this },
          async limit () {
            const needle = this.clauses[0].bindings[1].replace(/%/g, '')
            const candidate = { id: 1, title: 'Abyssum Design', path: 'games/abyssum' }
            return (candidate.title.toLowerCase().includes(needle) || candidate.path.toLowerCase().includes(needle)) ? [{ id: 1 }] : []
          }
        }) }
      }
    }
  })
  afterEach(() => { delete global.WIKI })
  const command = (revision, name, input = {}) => ({ projectId: 'project', revision, command: name, input: JSON.stringify(input) })
  test('JWT users without isActive are resolved against account status', async () => {
    expect((await resolver.Query.studioProject(null, { id: 'project' }, context(8))).canManage).toBe(true)
    await expect(resolver.Query.studioProject(null, { id: 'project' }, context(11))).rejects.toThrow('Unauthorized')
  })
  test('outsiders cannot read and readers cannot change projects', async () => {
    await expect(resolver.Query.studioProject(null, { id: 'project' }, context(12))).rejects.toThrow()
    await expect(resolver.Mutation.studioCommand(null, command(1, 'release', { name: 'Demo' }), context(9))).rejects.toThrow('Forbidden')
  })
  test('stale and future revisions cannot overwrite records', async () => {
    await expect(resolver.Mutation.studioCommand(null, command(2, 'release', { name: 'Demo' }), context(8))).rejects.toThrow('Reload')
    await resolver.Mutation.studioCommand(null, command(1, 'release', { name: 'Demo' }), context(8))
    await expect(resolver.Mutation.studioCommand(null, command(1, 'release', { name: 'Other' }), context(8))).rejects.toThrow('Reload')
    expect(JSON.parse(record.state).releases).toHaveLength(1)
  })
  test('editors cannot approve credits or grant access', async () => {
    await expect(resolver.Mutation.studioCommand(null, command(1, 'approveCredits'), context(10))).rejects.toThrow('Manager')
    await expect(resolver.Mutation.studioCommand(null, command(1, 'access'), context(10))).rejects.toThrow('Manager')
  })
  test('linked pages require page access as well as project access', async () => {
    await expect(resolver.Mutation.studioCommand(null, command(1, 'document', { pageId: 1 }), context(8))).rejects.toThrow('Page unavailable')
    pageAllowed = true
    await resolver.Mutation.studioCommand(null, command(1, 'document', { pageId: 1 }), context(8))
    pageAllowed = false
    const result = await resolver.Query.studioProject(null, { id: 'project' }, context(9))
    expect(JSON.parse(result.state).documents).toEqual([])
  })
  test('credit download requires project access', async () => {
    await expect(resolver.Query.studioCredits(null, { projectId: 'project', snapshotId: 'any', format: 'json' }, context(12))).rejects.toThrow()
  })
  test('unpublished page titles stay hidden from readers', async () => {
    pageAllowed = true
    await resolver.Mutation.studioCommand(null, command(1, 'document', { pageId: 1 }), context(8))
    pagePublished = false
    const result = await resolver.Query.studioProject(null, { id: 'project' }, context(9))
    expect(JSON.parse(result.state).documents).toEqual([])
  })
  test('API group access never inherits administrator ownership and records service identity', async () => {
    const service = { req: { user: { id: 1, apiId: 7, groups: [4], permissions: [] } } }
    await expect(resolver.Query.studioProject(null, { id: 'project' }, service)).rejects.toThrow()
    const state = JSON.parse(record.state)
    state.integrationAccess = [{ groupId: 4, role: 'editor' }]
    record.state = JSON.stringify(state)
    await resolver.Mutation.studioCommand(null, command(1, 'release', { name: 'API release' }), service)
    expect(JSON.parse(record.state).history[0].actor).toBe('api:7')
  })
  test('search queries match users, groups and pages case-insensitively across title and path', async () => {
    pageAllowed = true
    expect(await resolver.Query.studioUsers(null, { projectId: 'project', search: 'vox' }, context(8))).toEqual([{ id: 9, name: 'Vox Spectra' }])
    expect(await resolver.Query.studioGroups(null, { projectId: 'project', search: 'geppetto' }, context(8))).toEqual([{ id: 4, name: 'Geppetto Pipeline' }])
    expect(await resolver.Query.studioPages(null, { projectId: 'project', search: 'GAMES/ABYSSUM' }, context(8))).toEqual([{ id: 1, title: 'Abyssum Design', path: 'games/abyssum', locale: 'en' }])
  })
  test('managers receive resolved user and group names while non-managers have access lists redacted', async () => {
    const state = JSON.parse(record.state)
    state.integrationAccess = [{ groupId: 4, role: 'editor' }]
    record.state = JSON.stringify(state)
    const managerView = JSON.parse((await resolver.Query.studioProject(null, { id: 'project' }, context(8))).state)
    expect(managerView.access[0].name).toBe('User 9')
    expect(managerView.integrationAccess[0].name).toBe('Group 4')
    const editorView = JSON.parse((await resolver.Query.studioProject(null, { id: 'project' }, context(10))).state)
    expect(editorView.access).toEqual([])
    expect(editorView.integrationAccess).toEqual([])
  })
  test('enforces Tellurian Authentik group bindings, fail-closed partner isolation, and blueprint export', async () => {
    const authGroups = {
      20: { id: 20, name: 'Rogue Fairy Studios - C-Level' },
      21: { id: 21, name: 'Rogue Fairy Studios' },
      22: { id: 22, name: 'Rogue Fairy Studios - Partners' },
      23: { id: 23, name: 'Rogue Fairy Studios - GhostLore - Leads' },
      24: { id: 24, name: 'Rogue Fairy Studios - GhostLore - Partners' }
    }
    global.WIKI.auth.groups = authGroups
    global.WIKI.models.groups = {
      query: () => ({
        findById: async id => authGroups[id] || { id, name: `Group ${id}` }
      })
    }
    const memberCtx = (id, groups) => ({ req: { user: { id, groups, permissions: [] } } })

    // Editors cannot configure Lore namespace or Authentik group bindings
    await expect(resolver.Mutation.studioCommand(null, command(1, 'loreConfig', { projectSlug: 'ghostlore' }), context(10))).rejects.toThrow('Manager')
    await expect(resolver.Mutation.studioCommand(null, command(1, 'groupAccess', { groupId: 23, role: 'manager' }), context(10))).rejects.toThrow('Manager')

    // Manager configures internal Lore namespace with editor baseline for studio employees
    await resolver.Mutation.studioCommand(null, command(1, 'loreConfig', {
      projectSlug: 'ghostlore',
      visibility: 'internal',
      employeeRole: 'editor'
    }), context(8))
    await resolver.Mutation.studioCommand(null, command(2, 'groupAccess', {
      groupId: 23,
      role: 'manager',
      loreActions: ['read', 'write', 'admin', 'presign'],
      loreCanCreateRepo: true
    }), context(8))
    await resolver.Mutation.studioCommand(null, command(3, 'groupAccess', {
      groupId: 24,
      role: 'reader',
      loreActions: ['read'],
      loreCanCreateRepo: false
    }), context(8))

    // Studio employee (Rogue Fairy Studios) inherits baseline editor access on internal project
    const employeeProj = await resolver.Query.studioProject(null, { id: 'project' }, memberCtx(30, [21]))
    expect(employeeProj.canEdit).toBe(true)
    expect(employeeProj.canManage).toBe(false)
    expect(JSON.parse(employeeProj.state).groupAccess).toEqual([])
    expect(JSON.parse(employeeProj.state).loreBlueprint).toBeUndefined()

    // Fail-closed partner check: partner WITHOUT project group cannot access even if also in Rogue Fairy Studios
    await expect(resolver.Query.studioProject(null, { id: 'project' }, memberCtx(31, [22]))).rejects.toThrow('Project unavailable')
    await expect(resolver.Query.studioProject(null, { id: 'project' }, memberCtx(32, [21, 22]))).rejects.toThrow('Project unavailable')

    // Partner explicitly in GhostLore - Partners gets read-only access
    const partnerProj = await resolver.Query.studioProject(null, { id: 'project' }, memberCtx(33, [22, 24]))
    expect(partnerProj.canManage).toBe(false)
    expect(partnerProj.canEdit).toBe(false)

    // Project lead in GhostLore - Leads gets manager view with Authentik attributes and Blueprint YAML
    const leadProj = await resolver.Query.studioProject(null, { id: 'project' }, memberCtx(34, [21, 23]))
    expect(leadProj.canManage).toBe(true)
    const leadState = JSON.parse(leadProj.state)
    expect(leadState.groupAccess[0].authentikAttributes).toEqual({
      lore_project: 'ghostlore',
      lore_visibility: 'internal',
      lore_actions: ['read', 'write', 'admin', 'presign'],
      lore_can_create_repo: true
    })
    expect(leadState.loreBlueprint).toContain('name: "Rogue Fairy Studios - Lore Projects"')
    expect(leadState.loreBlueprint).toContain('name: "Rogue Fairy Studios - GhostLore - Leads"')
    expect(leadState.loreBlueprint).toContain('lore_project: ghostlore')

    // Switching visibility to restricted blocks baseline employees while C-Level retains full management
    await resolver.Mutation.studioCommand(null, command(4, 'loreConfig', {
      projectSlug: 'ghostlore',
      visibility: 'restricted'
    }), memberCtx(34, [21, 23]))
    await expect(resolver.Query.studioProject(null, { id: 'project' }, memberCtx(30, [21]))).rejects.toThrow('Project unavailable')
    const cLevelProj = await resolver.Query.studioProject(null, { id: 'project' }, memberCtx(35, [20]))
    expect(cLevelProj.canManage).toBe(true)
  })
})
