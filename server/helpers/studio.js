const crypto = require('crypto')

const states = ['backlog', 'ready', 'active', 'review', 'done']
const phases = ['pre_production', 'production', 'post_production', 'live']
const stages = ['concept', 'blockout', 'authoring', 'surface_loc', 'engine_integration', 'polish']
const tracks = ['general', 'art_2d', 'art_3d', 'narrative', 'spirit', 'scene_level', 'audio_vfx', 'code_system', 'typography', 'tabletop_print']
const tools = ['generic', 'goethe', 'geppetto', 'godot', 'carnifex', 'animafaber', 'fontpit', 'lore', 'urc', 'uefn', 'unity', 'git', 'github']
const uriProtocols = ['https:', 'http:', 'file:', 'goethe:', 'geppetto:', 'lore:', 'lores:', 'godot:', 'res:', 'uid:', 'carnifex:', 'animafaber:', 'fontpit:', 'uefn:', 'com.epicgames.launcher:']

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

function createState (name, kind, ownerId) {
  if (!['digital', 'analogue', 'hybrid'].includes(kind)) throw new Error('Invalid project type')
  return upgradeState({
    projectId: id(),
    name: text(name),
    kind,
    phase: 'pre_production',
    ownerId,
    access: [],
    participants: [],
    tasks: [],
    documents: [],
    releases: [],
    credits: [],
    builds: [],
    repositories: [],
    history: []
  })
}

const collections = [
  'access',
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
  'integrationEvents',
  'integrationAccess'
]

function upgradeState (state) {
  for (const collection of collections) {
    if (!Array.isArray(state[collection])) state[collection] = []
  }
  if (!phases.includes(state.phase)) state.phase = 'pre_production'
  for (const artifact of state.artifacts) {
    if (!stages.includes(artifact.stage)) artifact.stage = 'concept'
    if (!tracks.includes(artifact.track)) artifact.track = 'general'
    if (!tools.includes(artifact.tool)) artifact.tool = tools.includes(artifact.source) ? artifact.source : 'generic'
    if (artifact.toolUri === undefined) artifact.toolUri = null
    if (artifact.repoId === undefined) artifact.repoId = null
    if (artifact.assetPath === undefined) artifact.assetPath = ''
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
  stages,
  tracks,
  tools
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

module.exports = {
  createState,
  applyCommand,
  exportCredits,
  find,
  upgradeState,
  collections,
  phases,
  stages,
  tracks,
  tools,
  uriProtocols
}
