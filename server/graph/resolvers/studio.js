const crypto = require('crypto')
const studio = require('../../helpers/studio')
const templates = require('../../helpers/studio-templates')
const pageHelper = require('../../helpers/page')

/* global WIKI */

const { actor, roleFor, getProject, readablePage } = require('../../helpers/studio-access')

function searchPattern (raw) {
  return `%${String(raw || '').slice(0, 100).replace(/[%_]/g, '').toLowerCase()}%`
}

async function present (project, user) {
  const state = studio.upgradeState(JSON.parse(project.state))
  const role = roleFor(state, user)
  const documents = []
  for (const link of state.documents) {
    const page = await readablePage(link.pageId, user)
    if (page) documents.push({ ...link, title: page.title, path: page.path, locale: page.localeCode, outdated: link.revision !== page.updatedAt })
  }
  state.documents = documents
  if (role !== 'manager') {
    state.access = []
    state.integrationAccess = []
  } else {
    for (const entry of state.access) {
      const account = WIKI.models.users && WIKI.models.users.query ? await WIKI.models.users.query().findById(entry.userId) : null
      if (account && account.name) entry.name = account.name
    }
    for (const entry of state.integrationAccess) {
      const group = WIKI.models.groups && WIKI.models.groups.query ? await WIKI.models.groups.query().findById(entry.groupId) : null
      if (group && group.name) entry.name = group.name
    }
  }
  return { id: project.id, revision: project.revision, state: JSON.stringify(state), canManage: role === 'manager', canEdit: ['manager', 'editor'].includes(role) }
}

module.exports = {
  Query: {
    async studioTemplates (obj, args, context) {
      await actor(context)
      return templates.list(args.locale)
    },
    async studioGroups (obj, args, context) {
      const { role } = await getProject(args.projectId, await actor(context))
      if (role !== 'manager') throw new Error('Manager required')
      return WIKI.models.groups.query().select('id', 'name').whereRaw('LOWER(??) LIKE ?', ['name', searchPattern(args.search)]).orderBy('name').limit(30)
    },
    async studioUsers (obj, args, context) {
      const user = await actor(context)
      const { role } = await getProject(args.projectId, user)
      if (!['manager', 'editor'].includes(role)) throw new Error('Forbidden')
      return WIKI.models.users.query().select('id', 'name').where('isActive', true).whereNot('id', 2).whereRaw('LOWER(??) LIKE ?', ['name', searchPattern(args.search)]).orderBy('name').limit(30)
    },
    async studioPages (obj, args, context) {
      const user = await actor(context)
      await getProject(args.projectId, user)
      const pattern = searchPattern(args.search)
      const candidates = await WIKI.models.pages.query()
        .select('id')
        .where(builder => {
          builder.whereRaw('LOWER(??) LIKE ?', ['title', pattern]).orWhereRaw('LOWER(??) LIKE ?', ['path', pattern])
        })
        .orderBy('title')
        .limit(100)
      const results = []
      for (const candidate of candidates) {
        const page = await readablePage(candidate.id, user)
        if (page) results.push({ id: page.id, title: page.title, path: page.path, locale: page.localeCode })
      }
      return results.slice(0, 30)
    },
    async studioProjects (obj, args, context) {
      const user = await actor(context)
      const projects = await WIKI.models.studioProjects.query()
      return Promise.all(projects.filter(project => roleFor(JSON.parse(project.state), user)).map(project => present(project, user)))
    },
    async studioProject (obj, args, context) {
      const user = await actor(context)
      return present((await getProject(args.id, user)).project, user)
    },
    async studioCredits (obj, args, context) {
      const { state } = await getProject(args.projectId, await actor(context))
      return studio.exportCredits(studio.find(state.credits, args.snapshotId), args.format)
    }
  },
  Mutation: {
    async studioCreateProject (obj, args, context) {
      const user = await actor(context)
      if (!WIKI.auth.checkAccess(user, ['manage:system'])) throw new Error('Only administrators can create projects')
      const state = studio.createState(args.name, args.kind, user.apiId ? `api:${user.apiId}` : user.id)
      const project = await WIKI.models.studioProjects.query().insert({ id: state.projectId, revision: 1, state: JSON.stringify(state) })
      return present(project, user)
    },
    async studioCommand (obj, args, context) {
      const user = await actor(context)
      const actorId = user.apiId ? `api:${user.apiId}` : user.id
      const { project, state, role } = await getProject(args.projectId, user)
      if (project.revision !== args.revision) throw new Error('Project changed. Reload and retry your change.')
      if (!['manager', 'editor'].includes(role)) throw new Error('Forbidden')
      if (['access', 'integrationAccess', 'approveCredits', 'reviewArtifact', 'publishRelease', 'project'].includes(args.command) && role !== 'manager') throw new Error('Manager required')
      if (args.input.length > 500000) throw new Error('Input too large')
      const input = JSON.parse(args.input)
      if (!input || typeof input !== 'object' || Array.isArray(input)) throw new Error('Invalid input')
      if (state.archived && args.command !== 'project') throw new Error('Project is archived')
      if (args.command === 'participant' && input.userId) {
        const account = await WIKI.models.users.query().findById(input.userId)
        if (!account || !account.isActive || account.id === 2) throw new Error('Invalid linked account')
      }
      if (args.command === 'createDocument') {
        if (user.apiId) throw new Error('Create documents using a member account')
        if (typeof input.title !== 'string' || !input.title.trim() || input.title.length > 255 || typeof input.path !== 'string' || !input.path || input.path.length > 500 || typeof input.locale !== 'string') throw new Error('Invalid document title or path')
        const path = input.path.replace(/^\/+|\/+$/g, '')
        if (pageHelper.isReservedPath(path)) throw new Error('Reserved document path')
        const content = templates.render(input.template, input.locale, input.title)
        const projectTag = `studio-${state.projectId}`
        const lookup = () => WIKI.models.pages.query().findOne({ path, localeCode: input.locale }).withGraphFetched('tags')
        let page = await lookup()
        if (!page) {
          try {
            page = await WIKI.models.pages.createPage({ user, path, locale: input.locale, title: input.title.trim(), content, description: '', editor: 'markdown', isPublished: false, isPrivate: false, tags: [projectTag, `studio-template-${input.template}-v1`] })
          } catch (error) {
            page = await lookup()
            if (!page || !page.tags.some(tag => tag.tag === projectTag)) throw error
          }
        } else if (!page.tags.some(tag => tag.tag === projectTag)) throw new Error('Page path already belongs to another document')
        if (!await readablePage(page.id, user)) throw new Error('Document created; page permissions prevent linking it')
        if (!state.documents.some(link => link.pageId === page.id)) state.documents.push({ id: crypto.randomUUID(), pageId: page.id, revision: page.updatedAt, template: input.template, templateVersion: 1 })
        state.history.push({ action: 'createDocument', actor: actorId, at: new Date().toISOString(), template: input.template })
      } else if (args.command === 'integrationAccess') {
        const group = await WIKI.models.groups.query().findById(input.groupId)
        if (!group || !['reader', 'editor', 'remove'].includes(input.role)) throw new Error('Invalid integration group')
        state.integrationAccess = state.integrationAccess.filter(entry => entry.groupId !== group.id)
        if (input.role !== 'remove') state.integrationAccess.push({ id: crypto.randomUUID(), groupId: group.id, role: input.role })
        state.history.push({ action: 'integrationAccess', actor: actorId, at: new Date().toISOString(), groupId: group.id, role: input.role })
      } else if (args.command === 'access') {
        const target = await WIKI.models.users.query().findById(input.userId)
        if (!target || !target.isActive || target.id === 2 || state.ownerId === target.id) throw new Error('Invalid member')
        if (!['manager', 'editor', 'reader', 'remove'].includes(input.role)) throw new Error('Invalid role')
        state.access = state.access.filter(entry => entry.userId !== target.id)
        if (input.role !== 'remove') state.access.push({ userId: target.id, role: input.role })
        state.history.push({ action: 'access', actor: actorId, at: new Date().toISOString(), userId: target.id, role: input.role })
      } else if (args.command === 'refreshDocument') {
        const link = studio.find(state.documents, input.id)
        const page = await readablePage(link.pageId, user)
        if (!page) throw new Error('Page unavailable')
        state.history.push({ action: 'refreshDocument', actor: actorId, at: new Date().toISOString(), documentId: link.id, previousRevision: link.revision, revision: page.updatedAt })
        link.revision = page.updatedAt
      } else if (args.command === 'document') {
        const page = await readablePage(input.pageId, user)
        if (!page) throw new Error('Page unavailable')
        if (!state.documents.some(link => link.pageId === page.id)) state.documents.push({ id: crypto.randomUUID(), pageId: page.id, revision: page.updatedAt })
        state.history.push({ action: 'document', actor: actorId, at: new Date().toISOString() })
      } else {
        studio.applyCommand(state, args.command, input, actorId)
      }
      const changed = await WIKI.models.studioProjects.query().where({ id: args.projectId, revision: args.revision }).patch({ state: JSON.stringify(state), revision: args.revision + 1 })
      if (changed !== 1) throw new Error('Project changed. Reload and retry your change.')
      return present({ id: args.projectId, revision: args.revision + 1, state: JSON.stringify(state) }, user)
    }
  }
}
