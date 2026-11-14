const crypto = require('crypto')

const states = ['backlog', 'ready', 'active', 'review', 'done']
const phases = ['pre_production', 'production', 'post_production', 'live']
const engines = ['aetherion', 'godot', 'carnifex', 'uefn', 'unity', 'custom', 'tabletop']
const stages = ['concept', 'blockout', 'authoring', 'surface_loc', 'engine_integration', 'polish']
const tracks = ['general', 'art_2d', 'art_3d', 'narrative', 'spirit', 'scene_level', 'audio_vfx', 'code_system', 'typography', 'tabletop_print']
const tools = ['generic', 'goethe', 'geppetto', 'aetherion', 'godot', 'carnifex', 'animafaber', 'fontpit', 'lore', 'urc', 'uefn', 'unity', 'git', 'github']
const configTools = ['authentik', 'aetherion', 'geppetto', 'lore', 'goethe', 'carnifex', 'animafaber', 'fontpit', 'uri-handler', 'all']
const uriProtocols = ['https:', 'http:', 'file:', 'goethe:', 'geppetto:', 'lore:', 'lores:', 'aetherion:', 'godot:', 'res:', 'uid:', 'carnifex:', 'animafaber:', 'fontpit:', 'uefn:', 'com.epicgames.launcher:']
const loreActions = ['read', 'write', 'owner', 'admin', 'migrate', 'push-protected', 'obliterate', 'presign']
const loreVisibilities = ['internal', 'restricted']
const loreEmployeeRoles = ['none', 'reader', 'editor']
const authentikGroups = {
  cLevel: 'Rogue Fairy Studios - C-Level',
  admins: 'authentik Admins',
  employees: 'Rogue Fairy Studios',
  partners: 'Rogue Fairy Studios - Partners',
  loreCi: 'Rogue Fairy Studios - Lore CI',
  projectsRoot: 'Rogue Fairy Studios - Lore Projects'
}
const loreDefaults = {
  remoteBase: 'lores://lore.roguefairy.net',
  bridgeUrl: 'https://lore-auth.roguefairy.net',
  authUrl: 'https://auth.telluriancorp.info'
}

const id = () => crypto.randomUUID()
const digest = manifest => crypto.createHash('sha256').update(JSON.stringify(manifest)).digest('hex')

function hasControlChars (value) {
  for (let i = 0; i < value.length; i++) {
    const code = value.charCodeAt(i)
    if ((code >= 0 && code <= 8) || code === 11 || code === 12 || (code >= 14 && code <= 31) || code === 127) {
      return true
    }
  }
  return false
}

function text (value, maximum = 500) {
  if (typeof value !== 'string' || !value.trim() || value.length > maximum) throw new Error('Invalid text')
  if (hasControlChars(value)) throw new Error('Invalid control characters')
  return value.trim()
}

function safeUri (value) {
  const raw = text(value, 4000)
  const parsed = new URL(raw)
  if (!uriProtocols.includes(parsed.protocol)) throw new Error('Unsupported artifact location')
  if (parsed.username || parsed.password) throw new Error('Artifact location must not contain credentials')
  return parsed.href
}

function find (records, recordId) {
  const record = records.find(row => row.id === recordId)
  if (!record) throw new Error('Record not found')
  return record
}

function isValidLoreProjectSlug (value) {
  return typeof value === 'string' && value.length >= 1 && value.length <= 64 && /^[a-z0-9][a-z0-9._-]*$/.test(value)
}

function deriveLoreProjectSlug (name) {
  const clean = String(name || 'project')
    .toLowerCase()
    .replace(/[^a-z0-9._-]+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^[^a-z0-9]+|[-._]+$/g, '')
    .slice(0, 64)
  return isValidLoreProjectSlug(clean) ? clean : 'project'
}

function loreProjectSlug (value) {
  const clean = text(value, 64).toLowerCase()
  if (!isValidLoreProjectSlug(clean)) throw new Error('Invalid Lore project slug')
  return clean
}

function loreRepoPath (value) {
  const clean = text(value, 900).replace(/\\/g, '/').replace(/^\/+|\/+$/g, '')
  if (!clean) throw new Error('Invalid Lore repository name')
  for (const segment of clean.split('/')) {
    if (!segment || segment[0] === '.' || !/^[A-Za-z0-9._-]+$/.test(segment)) {
      throw new Error('Invalid Lore repository name')
    }
  }
  return clean
}

function parseLoreFullName (value) {
  const clean = text(value, 1000).replace(/\\/g, '/').replace(/^\/+|\/+$/g, '')
  const slash = clean.indexOf('/')
  if (slash <= 0 || slash === clean.length - 1) {
    throw new Error('Lore repository URI must use <project>/<repo> namespace')
  }
  const project = loreProjectSlug(clean.slice(0, slash))
  const repo = loreRepoPath(clean.slice(slash + 1))
  return { project, repo, fullName: `${project}/${repo}` }
}

function normalizeLoreActions (rawActions, fallbackRole = null) {
  if (rawActions === undefined || rawActions === null) {
    if (fallbackRole === 'manager') return ['read', 'write', 'admin', 'presign']
    if (fallbackRole === 'editor') return ['read', 'write']
    if (fallbackRole === 'reader') return ['read']
    return []
  }
  if (!Array.isArray(rawActions)) throw new Error('Invalid Lore actions')
  for (const action of rawActions) {
    if (!loreActions.includes(action)) throw new Error('Invalid Lore action')
  }
  return loreActions.filter(action => rawActions.includes(action))
}

function defaultEngineForKind (kind) {
  return kind === 'analogue' ? 'tabletop' : 'aetherion'
}

function createState (name, kind, ownerId, engine = '') {
  if (!['digital', 'analogue', 'hybrid'].includes(kind)) throw new Error('Invalid project type')
  const chosenEngine = engine ? String(engine).trim() : defaultEngineForKind(kind)
  if (!engines.includes(chosenEngine)) throw new Error('Invalid project engine')
  return upgradeState({
    projectId: id(),
    name: text(name),
    kind,
    engine: chosenEngine,
    phase: 'pre_production',
    ownerId,
    access: [],
    groupAccess: [],
    participants: [],
    tasks: [],
    documents: [],
    releases: [],
    credits: [],
    builds: [],
    repositories: [],
    platformTargets: [],
    history: []
  })
}

const collections = [
  'access',
  'groupAccess',
  'participants',
  'tasks',
  'documents',
  'releases',
  'credits',
  'builds',
  'history',
  'teams',
  'milestones',
  'cycles',
  'artifacts',
  'artifactVersions',
  'artifactReviews',
  'validations',
  'repositories',
  'platformTargets',
  'integrationEvents',
  'integrationAccess'
]

function upgradeState (state) {
  for (const collection of collections) {
    if (!Array.isArray(state[collection])) state[collection] = []
  }
  if (!phases.includes(state.phase)) state.phase = 'pre_production'
  if (!engines.includes(state.engine)) state.engine = defaultEngineForKind(state.kind)
  const currentLore = state.lore && typeof state.lore === 'object' ? state.lore : {}
  const projectSlug = isValidLoreProjectSlug(currentLore.projectSlug) ?
    currentLore.projectSlug :
    deriveLoreProjectSlug(state.name)
  const visibility = currentLore.visibility === 'restricted' ? 'restricted' : 'internal'
  const employeeRole = visibility === 'restricted' ?
    'none' :
    (loreEmployeeRoles.includes(currentLore.employeeRole) ? currentLore.employeeRole : 'none')
  state.lore = {
    projectSlug,
    visibility,
    employeeRole,
    remoteBase: typeof currentLore.remoteBase === 'string' && currentLore.remoteBase ? currentLore.remoteBase : loreDefaults.remoteBase,
    bridgeUrl: typeof currentLore.bridgeUrl === 'string' && currentLore.bridgeUrl ? currentLore.bridgeUrl : loreDefaults.bridgeUrl,
    authUrl: typeof currentLore.authUrl === 'string' && currentLore.authUrl ? currentLore.authUrl : loreDefaults.authUrl
  }
  for (const entry of state.groupAccess) {
    if (!Array.isArray(entry.loreActions)) {
      entry.loreActions = normalizeLoreActions(undefined, entry.role)
    } else {
      entry.loreActions = loreActions.filter(action => entry.loreActions.includes(action))
    }
    entry.loreCanCreateRepo = entry.loreCanCreateRepo === true
  }
  for (const repo of state.repositories) {
    if (repo.loreProject === undefined) repo.loreProject = ''
    if (repo.loreRepo === undefined) repo.loreRepo = ''
    if (repo.loreFullName === undefined) {
      repo.loreFullName = repo.loreProject && repo.loreRepo ? `${repo.loreProject}/${repo.loreRepo}` : ''
    }
    if (repo.system === 'lore' && !repo.loreFullName && repo.remoteUri) {
      try {
        const parsedPath = new URL(repo.remoteUri).pathname.replace(/^\/+|\/+$/g, '')
        if (parsedPath.includes('/')) {
          const parsedName = parseLoreFullName(parsedPath)
          repo.loreProject = parsedName.project
          repo.loreRepo = parsedName.repo
          repo.loreFullName = parsedName.fullName
        }
      } catch (err) {
        // Legacy single-segment or non-URL remote strings remain unchanged on upgrade
      }
    }
  }
  for (const artifact of state.artifacts) {
    if (!stages.includes(artifact.stage)) artifact.stage = 'concept'
    if (!tracks.includes(artifact.track)) artifact.track = 'general'
    if (!tools.includes(artifact.tool)) artifact.tool = tools.includes(artifact.source) ? artifact.source : 'generic'
    if (artifact.toolUri === undefined) artifact.toolUri = null
    if (artifact.repoId === undefined) artifact.repoId = null
    if (artifact.assetPath === undefined) artifact.assetPath = ''
  }
  for (const target of state.platformTargets) {
    if (!Array.isArray(target.checklist)) target.checklist = []
    if (!Array.isArray(target.assets)) target.assets = []
  }
  return state
}

function entriesFor (state, participantIds) {
  if (!Array.isArray(participantIds) || !participantIds.length) throw new Error('Select contributors')
  const entries = []
  for (const participantId of new Set(participantIds)) {
    const person = find(state.participants, participantId)
    const team = state.teams.find(team => team.id === person.teamId)
    for (const role of person.roles) {
      entries.push({ participantId, name: person.name, role, section: team ? team.name : person.team || 'Team', order: person.order })
    }
  }
  return entries.sort((a, b) => a.order - b.order || compare(a.section, b.section) || compare(a.participantId, b.participantId) || compare(a.role, b.role))
}

function compare (a, b) { return a < b ? -1 : a > b ? 1 : 0 }

const commands = {
  participant (state, input) {
    if (!Array.isArray(input.roles) || !input.roles.length || input.roles.length > 30) throw new Error('Specify credit roles')
    const roles = [...new Set(input.roles.map(role => text(role, 120)))]
    const order = input.order || 0
    if (!Number.isSafeInteger(order)) throw new Error('Invalid order')
    const team = input.teamId ? find(state.teams, input.teamId) : null
    if (team && team.archived) throw new Error('Team is archived')
    const userId = input.userId || null
    if (userId !== null && (!Number.isInteger(userId) || userId < 1 || userId === 2)) throw new Error('Invalid linked account')
    const data = { name: text(input.name, 255), roles, team: team ? team.name : input.team ? text(input.team, 120) : '', teamId: team ? team.id : null, userId, order }
    if (input.id) Object.assign(find(state.participants, input.id), data)
    else state.participants.push({ id: id(), ...data, startedAt: new Date().toISOString(), endedAt: null })
  },
  endParticipation (state, input) { find(state.participants, input.id).endedAt = new Date().toISOString() },
  task (state, input) {
    if (input.assigneeId && find(state.participants, input.assigneeId).endedAt) throw new Error('Participant is inactive')
    state.tasks.push({ id: id(), title: text(input.title), criteria: input.criteria ? text(input.criteria, 10000) : '', discipline: input.discipline ? text(input.discipline, 100) : '', assigneeId: input.assigneeId || null, status: 'backlog' })
  },
  transition (state, input) {
    if (!states.includes(input.status)) throw new Error('Invalid task state')
    const task = find(state.tasks, input.id)
    if (input.status === 'done' && (task.blockedReason || (task.dependencies || []).some(dependency => find(state.tasks, dependency).status !== 'done'))) throw new Error('Resolve task blockers first')
    task.status = input.status
  },
  release (state, input) { state.releases.push({ id: id(), name: text(input.name) }) },
  generateCredits (state, input) {
    const release = find(state.releases, input.releaseId)
    const manifest = { schemaVersion: 1, projectId: state.projectId, releaseId: release.id, title: release.name, entries: entriesFor(state, input.participantIds) }
    const checksum = digest(manifest)
    if (!state.credits.some(row => row.releaseId === release.id && row.checksum === checksum)) {
      state.credits.push({ id: id(), releaseId: release.id, status: 'draft', checksum, manifest, createdAt: new Date().toISOString() })
    }
  },
  editCredits (state, input) {
    const snapshot = find(state.credits, input.id)
    if (snapshot.status !== 'draft') throw new Error('Approved credits are immutable')
    if (!Array.isArray(input.entries) || !input.entries.length || input.entries.length > 5000) throw new Error('Invalid credit entries')
    text(input.reason)
    const entries = input.entries.map((entry, order) => ({
      participantId: find(state.participants, entry.participantId).id,
      name: text(entry.name, 255),
      role: text(entry.role, 120),
      section: text(entry.section, 120),
      order
    }))
    const keys = entries.map(entry => JSON.stringify([entry.participantId, entry.role, entry.section]))
    if (new Set(keys).size !== keys.length) throw new Error('Duplicate credit')
    snapshot.manifest.entries = entries
    snapshot.checksum = digest(snapshot.manifest)
  },
  reviseCredits (state, input) {
    const snapshot = find(state.credits, input.id)
    if (snapshot.status !== 'approved') throw new Error('Select approved credits')
    state.credits.push({ id: id(), releaseId: snapshot.releaseId, status: 'draft', checksum: snapshot.checksum, manifest: JSON.parse(JSON.stringify(snapshot.manifest)), previousId: snapshot.id, createdAt: new Date().toISOString() })
  },
  approveCredits (state, input, actor) {
    const snapshot = find(state.credits, input.id)
    if (snapshot.status !== 'draft') throw new Error('Credits already approved')
    Object.assign(snapshot, { status: 'approved', approvedBy: actor, approvedAt: new Date().toISOString() })
  },
  build (state, input) {
    const release = find(state.releases, input.releaseId)
    if (release.publishedAt) throw new Error('Published release is immutable')
    const snapshot = find(state.credits, input.snapshotId)
    if (snapshot.status !== 'approved' || snapshot.releaseId !== release.id) throw new Error('Select approved credits for this release')
    const name = text(input.name)
    let uri = null
    if (input.uri) {
      try {
        uri = safeUri(input.uri)
      } catch (err) {
        throw new Error('Invalid build location')
      }
    }
    if (input.checksum && !/^[a-f0-9]{64}$/i.test(input.checksum)) throw new Error('Invalid build checksum')
    const artifactChecksum = input.checksum ? input.checksum.toLowerCase() : null
    const vcsRevision = input.vcsRevision ? text(input.vcsRevision, 200) : null
    const existing = state.builds.find(build => build.releaseId === release.id && build.name === name)
    if (existing) {
      if (existing.snapshotId !== snapshot.id || (existing.uri || null) !== uri || (existing.artifactChecksum ? existing.artifactChecksum.toLowerCase() : null) !== artifactChecksum) throw new Error('Build identifier already has different content or credits')
      return
    }
    state.builds.push({ id: id(), name, releaseId: release.id, snapshotId: snapshot.id, checksum: snapshot.checksum, artifactChecksum, uri, vcsRevision })
  }
}

Object.assign(commands, require('./studio-production')({
  text,
  find,
  id,
  build: commands.build,
  safeUri,
  phases,
  engines,
  stages,
  tracks,
  tools,
  loreProjectSlug,
  loreRepoPath,
  parseLoreFullName,
  loreVisibilities,
  loreEmployeeRoles,
  loreDefaults
}))

function applyCommand (state, command, input, actor) {
  upgradeState(state)
  if (state.archived && command !== 'project') throw new Error('Project is archived')
  if (!Object.prototype.hasOwnProperty.call(commands, command)) throw new Error('Unknown command')
  commands[command](state, input, actor)
  state.history.push({ id: id(), action: command, actor, at: new Date().toISOString(), reason: input.reason || null, input: JSON.parse(JSON.stringify(input)) })
  return state
}

function exportCredits (snapshot, format) {
  if (snapshot.status !== 'approved') throw new Error('Credits must be approved')
  if (format === 'json') return JSON.stringify(snapshot.manifest)
  if (format !== 'markdown') throw new Error('Unsupported format')
  const escape = value => value.replace(/[\\`*_{}[\]()#+.!|<>\r\n]/g, character => '&#' + character.charCodeAt(0) + ';')
  return '# ' + escape(snapshot.manifest.title) + '\n\n' + snapshot.manifest.entries.map(entry => '- ' + escape(entry.section) + ' / ' + escape(entry.role) + ' — ' + escape(entry.name)).join('\n') + '\n'
}

function exportAuthentikAttributes (state, entry) {
  upgradeState(state)
  const actions = loreActions.filter(action => (entry.loreActions || []).includes(action))
  return {
    lore_project: state.lore.projectSlug,
    lore_visibility: state.lore.visibility,
    lore_actions: actions,
    lore_can_create_repo: entry.loreCanCreateRepo === true
  }
}

function yamlScalar (value) {
  const str = String(value || '')
  if (/^[A-Za-z0-9._/-]+$/.test(str) && !['true', 'false', 'null', 'yes', 'no', 'on', 'off'].includes(str.toLowerCase())) {
    return str
  }
  return JSON.stringify(str)
}

function exportAuthentikBlueprint (state) {
  upgradeState(state)
  const lines = [
    '# Tellurian Authentik Blueprint — Rogue Fairy Studios Lore Project Groups',
    '# Target: ' + state.lore.authUrl,
    'version: 1',
    'metadata:',
    '  name: ' + yamlScalar(`Rogue Fairy Studios - Lore - ${state.name}`),
    'entries:',
    '  - model: authentik_core.group',
    '    id: group-rfs-projects-root',
    '    identifiers:',
    '      name: ' + yamlScalar(authentikGroups.projectsRoot)
  ]
  for (const entry of state.groupAccess) {
    const groupLabel = entry.name || entry.groupName || `Rogue Fairy Studios - ${state.name} - ${entry.role}`
    const entrySlug = deriveLoreProjectSlug(`${state.lore.projectSlug}-${groupLabel}`).replace(/\./g, '-')
    const attrs = exportAuthentikAttributes(state, entry)
    lines.push(
      '  - model: authentik_core.group',
      `    id: group-project-${entrySlug}`,
      '    identifiers:',
      '      name: ' + yamlScalar(groupLabel),
      '    attrs:',
      '      parent: !KeyOf group-rfs-projects-root',
      '      attributes:',
      '        lore_project: ' + yamlScalar(attrs.lore_project),
      '        lore_visibility: ' + yamlScalar(attrs.lore_visibility)
    )
    if (attrs.lore_actions.length) {
      lines.push('        lore_actions:')
      for (const action of attrs.lore_actions) {
        lines.push('          - ' + action)
      }
    } else {
      lines.push('        lore_actions: []')
    }
    lines.push('        lore_can_create_repo: ' + (attrs.lore_can_create_repo ? 'true' : 'false'))
  }
  return lines.join('\n') + '\n'
}

module.exports = {
  createState,
  applyCommand,
  exportCredits,
  exportAuthentikAttributes,
  exportAuthentikBlueprint,
  normalizeLoreActions,
  deriveLoreProjectSlug,
  isValidLoreProjectSlug,
  parseLoreFullName,
  find,
  upgradeState,
  defaultEngineForKind,
  collections,
  phases,
  engines,
  stages,
  tracks,
  tools,
  configTools,
  uriProtocols,
  loreActions,
  loreVisibilities,
  loreEmployeeRoles,
  authentikGroups,
  loreDefaults
}
