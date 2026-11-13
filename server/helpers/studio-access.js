const studio = require('./studio')

/* global WIKI */

async function actor (context) {
  const user = context.req.user
  if (user && user.apiId) return user
  if (!user || !Number.isInteger(user.id) || user.id === 2) throw new Error('Unauthorized')
  const account = await WIKI.models.users.query().findById(user.id)
  if (!account || !account.isActive) throw new Error('Unauthorized')
  return user
}

function roleFor (state, user) {
  if (user.apiId) {
    if (WIKI.auth.checkAccess(user, ['manage:system'])) return 'manager'
    const integration = (state.integrationAccess || []).find(entry => user.groups.includes(entry.groupId))
    return integration ? integration.role : null
  }
  if (state.ownerId === user.id || WIKI.auth.checkAccess(user, ['manage:system'])) return 'manager'
  const access = state.access.find(entry => entry.userId === user.id)
  return access ? access.role : null
}

async function getProject (id, user) {
  const project = await WIKI.models.studioProjects.query().findById(id)
  if (!project) throw new Error('Project unavailable')
  const state = studio.upgradeState(JSON.parse(project.state))
  const role = roleFor(state, user)
  if (!role) throw new Error('Project unavailable')
  return { project, state, role }
}

async function readablePage (pageId, user) {
  const page = await WIKI.models.pages.query().findById(pageId).withGraphFetched('tags')
  if (!page || !WIKI.auth.checkAccess(user, ['read:pages'], { path: page.path, locale: page.localeCode, tags: page.tags, isPrivate: page.isPrivate })) return null
  if (page.isPrivate && page.authorId !== user.id && !WIKI.auth.checkAccess(user, ['manage:system'])) return null
  const now = Date.now()
  const published = page.isPublished && (!page.publishStartDate || new Date(page.publishStartDate).getTime() <= now) && (!page.publishEndDate || new Date(page.publishEndDate).getTime() >= now)
  if (!published && !WIKI.auth.checkAccess(user, ['write:pages'], { path: page.path, locale: page.localeCode, tags: page.tags })) return null
  return page
}

module.exports = { actor, roleFor, getProject, readablePage }
