const studio = require('./studio')

/* global WIKI */

const roleOrder = { reader: 1, editor: 2, manager: 3 }

function pickHigherRole (current, candidate) {
  if (!candidate || !roleOrder[candidate]) return current
  if (!current || roleOrder[candidate] > roleOrder[current]) return candidate
  return current
}

function userGroupContext (user) {
  const ids = new Set()
  const names = new Set()
  const authGroups = (typeof WIKI !== 'undefined' && WIKI.auth && WIKI.auth.groups) || {}
  const list = user && Array.isArray(user.groups) ? user.groups : []
  for (const item of list) {
    if (Number.isInteger(item)) {
      ids.add(item)
      if (authGroups[item] && authGroups[item].name) names.add(authGroups[item].name)
    } else if (item && typeof item === 'object') {
      if (Number.isInteger(item.id)) {
        ids.add(item.id)
        if (authGroups[item.id] && authGroups[item.id].name) names.add(authGroups[item.id].name)
      }
      if (typeof item.name === 'string' && item.name) names.add(item.name)
    }
  }
  return { ids, names }
}

async function actor (context) {
  const user = context.req.user
  if (user && user.apiId) return user
  if (!user || !Number.isInteger(user.id) || user.id === 2) throw new Error('Unauthorized')
  const account = await WIKI.models.users.query().findById(user.id)
  if (!account || !account.isActive) throw new Error('Unauthorized')
  return user
}

function roleFor (state, user) {
  const groups = userGroupContext(user)
  if (user.apiId) {
    if (WIKI.auth.checkAccess(user, ['manage:system'])) return 'manager'
    let apiRole = null
    for (const entry of (state.integrationAccess || [])) {
      if (groups.ids.has(entry.groupId)) apiRole = pickHigherRole(apiRole, entry.role)
    }
    return apiRole
  }
  if (
    state.ownerId === user.id ||
    WIKI.auth.checkAccess(user, ['manage:system']) ||
    groups.names.has(studio.authentikGroups.cLevel) ||
    groups.names.has(studio.authentikGroups.admins)
  ) {
    return 'manager'
  }
  let role = null
  const access = (state.access || []).find(entry => entry.userId === user.id)
  if (access) role = pickHigherRole(role, access.role)
  for (const entry of (state.groupAccess || [])) {
    if (groups.ids.has(entry.groupId) || (entry.groupName && groups.names.has(entry.groupName))) {
      role = pickHigherRole(role, entry.role)
    }
  }
  const lore = state.lore || {}
  const visibility = lore.visibility === 'restricted' ? 'restricted' : 'internal'
  const isPartner = groups.names.has(studio.authentikGroups.partners)
  const isEmployee = groups.names.has(studio.authentikGroups.employees)
  if (visibility === 'internal' && !isPartner && isEmployee && ['reader', 'editor'].includes(lore.employeeRole)) {
    role = pickHigherRole(role, lore.employeeRole)
  }
  return role
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
