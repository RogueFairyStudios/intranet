const crypto = require('crypto')

module.exports = ({ text, find, id, build }) => {
  function reference (rows, value) { return value ? find(rows, value).id : null }
  function references (rows, values = []) {
    if (!Array.isArray(values)) throw new Error('Expected references')
    return [...new Set(values.map(value => find(rows, value).id))]
  }
  function date (value) {
    if (!value) return null
    if (typeof value !== 'string' || !Number.isFinite(Date.parse(value))) throw new Error('Invalid date')
    return new Date(value).toISOString()
  }
  function save (rows, input, values) {
    if (input.id) Object.assign(find(rows, input.id), values)
    else rows.push({ id: id(), ...values })
  }
  function uri (value) {
    const parsed = new URL(text(value, 4000))
    if (!['https:', 'http:', 'file:'].includes(parsed.protocol)) throw new Error('Unsupported artifact location')
    if (parsed.username || parsed.password) throw new Error('Artifact location must not contain credentials')
    return parsed.href
  }
  function approved (state, versionId) {
    const reviews = state.artifactReviews.filter(row => row.versionId === versionId)
    return reviews.length > 0 && reviews[reviews.length - 1].decision === 'approved'
  }
  const commands = {
    build,
    project (state, input) {
      if (!['digital', 'analogue', 'hybrid'].includes(input.kind)) throw new Error('Invalid project type')
      state.name = text(input.name)
      state.kind = input.kind
      state.archived = input.archived === true
    },
    team (state, input) {
      save(state.teams, input, { name: text(input.name, 120), archived: input.archived === true })
    },
    milestone (state, input) {
      save(state.milestones, input, { name: text(input.name), criteria: input.criteria ? text(input.criteria, 10000) : '', dueDate: date(input.dueDate) })
    },
    cycle (state, input) {
      const startDate = date(input.startDate)
      const endDate = date(input.endDate)
      if (startDate && endDate && startDate > endDate) throw new Error('Cycle ends before it starts')
      const capacity = input.capacity || 0
      if (!Number.isFinite(capacity) || capacity < 0) throw new Error('Invalid capacity')
      save(state.cycles, input, { name: text(input.name), goal: input.goal ? text(input.goal, 10000) : '', startDate, endDate, capacity })
    },
    artifact (state, input) {
      save(state.artifacts, input, { name: text(input.name), kind: text(input.kind, 120), taskId: reference(state.tasks, input.taskId), documentIds: references(state.documents, input.documentIds) })
    },
    artifactVersion (state, input) {
      let artifactId = input.artifactId
      if (!artifactId && input.externalId) {
        const artifact = state.artifacts.find(row => row.externalId === input.externalId && row.source === input.source)
        if (!artifact) throw new Error('External artifact not registered')
        artifactId = artifact.id
      }
      find(state.artifacts, artifactId)
      const version = text(input.version, 120)
      if (!/^[a-f0-9]{64}$/i.test(input.checksum)) throw new Error('Expected SHA-256 checksum')
      const data = { artifactId, version, uri: uri(input.uri), checksum: input.checksum.toLowerCase(), previewUri: input.previewUri ? uri(input.previewUri) : null }
      const existing = state.artifactVersions.find(row => row.artifactId === artifactId && row.version === version)
      if (existing) {
        if (existing.checksum !== data.checksum || existing.uri !== data.uri || existing.previewUri !== data.previewUri) throw new Error('Artifact version already exists with different content')
        return
      }
      state.artifactVersions.push({ id: id(), ...data, createdAt: new Date().toISOString() })
    },
    reviewArtifact (state, input, actor) {
      find(state.artifactVersions, input.versionId)
      if (!['approved', 'changes'].includes(input.decision)) throw new Error('Invalid review decision')
      state.artifactReviews.push({ id: id(), versionId: input.versionId, decision: input.decision, comment: input.comment ? text(input.comment, 10000) : '', actor, at: new Date().toISOString() })
    },
    validation (state, input, actor) {
      if (!['test', 'playtest'].includes(input.kind) || !['passed', 'failed', 'pending'].includes(input.result)) throw new Error('Invalid validation')
      state.validations.push({ id: id(), name: text(input.name), kind: input.kind, result: input.result, notes: input.notes ? text(input.notes, 20000) : '', releaseId: reference(state.releases, input.releaseId), artifactVersionId: reference(state.artifactVersions, input.artifactVersionId), taskId: reference(state.tasks, input.taskId), actor, at: new Date().toISOString() })
    },
    release (state, input) {
      const existing = input.id ? find(state.releases, input.id) : null
      if (existing && existing.publishedAt) throw new Error('Published release is immutable')
      save(state.releases, input, { name: text(input.name), criteria: input.criteria ? text(input.criteria, 10000) : '', artifactVersionIds: references(state.artifactVersions, input.artifactVersionIds), validationIds: references(state.validations, input.validationIds), publishedAt: null })
    },
    publishRelease (state, input, actor) {
      const release = find(state.releases, input.id)
      if (release.publishedAt) throw new Error('Release already published')
      const builds = state.builds.filter(build => build.releaseId === release.id)
      if (!builds.length) throw new Error('Register a build or prototype with approved credits')
      if (!release.criteria) throw new Error('Specify release acceptance criteria')
      if (!release.validationIds || !release.validationIds.length) throw new Error('Select validation evidence')
      for (const validationId of release.validationIds) {
        const validation = find(state.validations, validationId)
        if (validation.result !== 'passed') throw new Error('Selected validation has not passed')
        if (validation.releaseId && validation.releaseId !== release.id) throw new Error('Validation belongs to another release')
        if (validation.artifactVersionId && !release.artifactVersionIds.includes(validation.artifactVersionId)) throw new Error('Validation refers to a different artifact version')
      }
      for (const versionId of release.artifactVersionIds || []) {
        if (!approved(state, versionId)) throw new Error('Artifact version requires approval')
      }
      release.publishedAt = new Date().toISOString()
      release.publishedBy = actor
      release.buildIds = builds.map(build => build.id)
      release.reviewIds = (release.artifactVersionIds || []).map(versionId => state.artifactReviews.filter(review => review.versionId === versionId).slice(-1)[0].id)
    },
    task (state, input) {
      const existing = input.id ? find(state.tasks, input.id) : null
      const assigneeId = reference(state.participants, input.assigneeId)
      if (assigneeId && find(state.participants, assigneeId).endedAt && (!existing || existing.assigneeId !== assigneeId)) throw new Error('Participant is inactive')
      const dependencies = references(state.tasks, input.dependencies)
      const taskId = input.id || id()
      const visit = (dependency, seen = new Set()) => {
        if (dependency === taskId) throw new Error('Task dependency cycle')
        if (seen.has(dependency)) return
        seen.add(dependency)
        for (const next of find(state.tasks, dependency).dependencies || []) visit(next, seen)
      }
      dependencies.forEach(dependency => visit(dependency))
      const priority = input.priority || 'normal'
      const type = input.type || 'task'
      if (!['low', 'normal', 'high', 'urgent'].includes(priority) || !['task', 'bug', 'feature'].includes(type)) throw new Error('Invalid task priority or type')
      const values = { title: text(input.title), criteria: input.criteria ? text(input.criteria, 10000) : '', discipline: input.discipline ? text(input.discipline, 100) : '', assigneeId, priority, type, dueDate: date(input.dueDate), blockedReason: input.blockedReason ? text(input.blockedReason, 2000) : '', dependencies, milestoneId: reference(state.milestones, input.milestoneId), cycleId: reference(state.cycles, input.cycleId), documentIds: references(state.documents, input.documentIds) }
      if (existing) Object.assign(existing, values)
      else state.tasks.push({ id: taskId, ...values, status: 'backlog' })
    }
  }
  commands.integrationEvent = (state, input, actor) => {
    const source = text(input.source, 50)
    if (!['geppetto', 'goethe', 'godot', 'unity', 'carnifex', 'generic', 'github'].includes(source)) throw new Error('Unsupported source')
    const eventId = text(input.eventId, 200)
    const canonical = value => {
      if (Array.isArray(value)) return value.map(canonical)
      if (value && typeof value === 'object') return Object.fromEntries(Object.keys(value).sort().map(key => [key, canonical(value[key])]))
      return value
    }
    const fingerprint = crypto.createHash('sha256').update(JSON.stringify(canonical({ type: input.type, payload: input.payload }))).digest('hex')
    const existing = state.integrationEvents.find(event => event.source === source && event.eventId === eventId)
    if (existing) {
      if (existing.fingerprint !== fingerprint) throw new Error('Event ID reused with different content')
      return
    }
    if (!['artifact', 'artifactVersion', 'build', 'validation'].includes(input.type)) throw new Error('Unsupported event type')
    const payload = { ...input.payload, source }
    if (input.type === 'artifact') {
      const externalId = text(payload.externalId, 500)
      const artifact = state.artifacts.find(row => row.source === source && row.externalId === externalId)
      if (artifact) payload.id = artifact.id
      commands.artifact(state, payload)
      const updated = payload.id ? find(state.artifacts, payload.id) : state.artifacts[state.artifacts.length - 1]
      Object.assign(updated, { source, externalId })
    } else {
      commands[input.type](state, payload, actor)
    }
    state.integrationEvents.push({ id: id(), source, eventId, fingerprint, type: input.type, at: new Date().toISOString() })
  }
  return commands
}
