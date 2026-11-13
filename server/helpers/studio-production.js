const crypto = require('crypto')
const {
  storefronts,
  submissionStatuses,
  itemStatuses,
  checkCategories,
  buildPreset,
  evaluateTargetReadiness
} = require('./studio-platforms')

const vcsSystems = ['git', 'lore', 'urc']
const repoProtocols = ['https:', 'http:', 'ssh:', 'git:', 'lore:', 'lores:', 'urc:', 'urcs:', 'uefn:', 'com.epicgames.launcher:']

module.exports = ({ text, find, id, build, safeUri, phases, stages, tracks, tools }) => {
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
    return safeUri(value)
  }
  function phase (value, fallback = '') {
    if (!value) return fallback
    if (!phases.includes(value)) throw new Error('Invalid production phase')
    return value
  }
  function stage (value, fallback = '') {
    if (!value) return fallback
    if (!stages.includes(value)) throw new Error('Invalid pipeline stage')
    return value
  }
  function track (value, fallback = 'general') {
    if (!value) return fallback
    if (!tracks.includes(value)) throw new Error('Invalid pipeline track')
    return value
  }
  function tool (value, fallback = 'generic') {
    if (!value) return fallback
    if (!tools.includes(value)) throw new Error('Invalid studio tool')
    return value
  }
  function slug (value) {
    const clean = text(value, 120).toLowerCase()
    if (!/^[a-z0-9][a-z0-9._-]*$/.test(clean)) throw new Error('Invalid repository slug')
    return clean
  }
  function repoPath (value) {
    if (!value) return ''
    const clean = text(value, 1000).replace(/\\/g, '/').replace(/^\/+/, '')
    if (clean.split('/').includes('..')) throw new Error('Invalid asset path')
    return clean
  }
  function remoteUri (value) {
    if (!value) return ''
    const raw = text(value, 2000)
    if (/^git@[A-Za-z0-9._-]+:[A-Za-z0-9._/-]+(?:\.git)?$/.test(raw)) return raw
    const parsed = new URL(raw)
    if (!repoProtocols.includes(parsed.protocol)) throw new Error('Unsupported repository remote URI')
    if (parsed.password || (parsed.username && !(parsed.protocol === 'ssh:' && parsed.username === 'git'))) {
      throw new Error('Repository URI must not contain credentials')
    }
    return parsed.href
  }
  function count (value) {
    if (value === undefined || value === null || value === '') return 0
    if (!Number.isInteger(value) || value < 0 || value > 1000000) throw new Error('Invalid count')
    return value
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
      if (input.phase) state.phase = phase(input.phase)
    },
    team (state, input) {
      save(state.teams, input, { name: text(input.name, 120), archived: input.archived === true })
    },
    milestone (state, input) {
      const existing = input.id ? find(state.milestones, input.id) : null
      save(state.milestones, input, {
        name: text(input.name),
        phase: phase(input.phase, (existing && existing.phase) || state.phase || 'pre_production'),
        criteria: input.criteria ? text(input.criteria, 10000) : '',
        dueDate: date(input.dueDate)
      })
    },
    cycle (state, input) {
      const startDate = date(input.startDate)
      const endDate = date(input.endDate)
      if (startDate && endDate && startDate > endDate) throw new Error('Cycle ends before it starts')
      const capacity = input.capacity || 0
      if (!Number.isFinite(capacity) || capacity < 0) throw new Error('Invalid capacity')
      save(state.cycles, input, { name: text(input.name), goal: input.goal ? text(input.goal, 10000) : '', startDate, endDate, capacity })
    },
    repository (state, input) {
      const existing = input.id ? find(state.repositories, input.id) : null
      if (!vcsSystems.includes(input.system)) throw new Error('Invalid VCS system')
      const repoSlug = slug(input.slug || input.name.toLowerCase().replace(/[^a-z0-9._-]+/g, '-').replace(/^-+|-+$/g, ''))
      const duplicate = state.repositories.find(row => row.slug === repoSlug && (!existing || row.id !== existing.id))
      if (duplicate) throw new Error('Repository slug already exists')
      const values = {
        name: text(input.name, 200),
        slug: repoSlug,
        system: input.system,
        remoteUri: remoteUri(input.remoteUri),
        defaultBranch: input.defaultBranch ? text(input.defaultBranch, 200) : 'main',
        workspacePath: repoPath(input.workspacePath),
        status: existing ? existing.status || null : null
      }
      save(state.repositories, input, values)
    },
    vcsStatus (state, input, actor) {
      let repo = null
      if (input.repositoryId) {
        repo = find(state.repositories, input.repositoryId)
      } else if (input.slug) {
        const repoSlug = slug(input.slug)
        repo = state.repositories.find(row => row.slug === repoSlug)
        if (!repo) {
          const sys = vcsSystems.includes(input.system) ? input.system : 'git'
          repo = {
            id: id(),
            name: input.name ? text(input.name, 200) : repoSlug,
            slug: repoSlug,
            system: sys,
            remoteUri: remoteUri(input.remoteUri),
            defaultBranch: input.defaultBranch ? text(input.defaultBranch, 200) : (input.branch ? text(input.branch, 200) : 'main'),
            workspacePath: repoPath(input.workspacePath),
            status: null
          }
          state.repositories.push(repo)
        }
      } else {
        throw new Error('Specify repositoryId or slug')
      }
      if (input.remoteUri) repo.remoteUri = remoteUri(input.remoteUri)
      const modifiedFiles = Array.isArray(input.modifiedFiles) ?
        input.modifiedFiles.slice(0, 100).map(repoPath).filter(Boolean) :
        []
      const locks = Array.isArray(input.locks) ?
        input.locks.slice(0, 200).map(lock => ({
          path: repoPath(lock.path),
          owner: text(lock.owner, 200),
          branch: lock.branch ? text(lock.branch, 200) : '',
          lockedAt: date(lock.lockedAt)
        })).filter(lock => lock.path) :
        []
      repo.status = {
        branch: input.branch ? text(input.branch, 200) : repo.defaultBranch || 'main',
        revision: input.revision ? text(input.revision, 200) : '',
        revisionNumber: Number.isSafeInteger(input.revisionNumber) && input.revisionNumber >= 0 ? input.revisionNumber : null,
        commitMessage: input.commitMessage ? text(input.commitMessage, 1000) : '',
        author: input.author ? text(input.author, 200) : '',
        clean: input.clean !== false && modifiedFiles.length === 0 && count(input.modifiedCount) === 0 && count(input.stagedCount) === 0,
        modifiedCount: count(input.modifiedCount !== undefined ? input.modifiedCount : modifiedFiles.length),
        stagedCount: count(input.stagedCount),
        untrackedCount: count(input.untrackedCount),
        ahead: count(input.ahead),
        behind: count(input.behind),
        modifiedFiles,
        locks,
        compression: input.compression ? text(input.compression, 40) : '',
        updatedAt: new Date().toISOString(),
        reportedBy: String(actor)
      }
    },
    artifact (state, input) {
      const existing = input.id ? find(state.artifacts, input.id) : null
      const defaultTool = (existing && existing.tool) || (input.source && tools.includes(input.source) ? input.source : 'generic')
      const resolvedToolUri = input.toolUri ?
        uri(input.toolUri) :
        (input.toolUri === '' ? null : (existing ? existing.toolUri || null : null))
      save(state.artifacts, input, {
        name: text(input.name),
        kind: text(input.kind, 120),
        track: track(input.track, (existing && existing.track) || 'general'),
        stage: stage(input.stage, (existing && existing.stage) || 'concept'),
        tool: tool(input.tool, defaultTool),
        toolUri: resolvedToolUri,
        repoId: input.repoId !== undefined ? reference(state.repositories, input.repoId) : (existing ? existing.repoId || null : null),
        assetPath: input.assetPath !== undefined ? repoPath(input.assetPath) : (existing ? existing.assetPath || '' : ''),
        taskId: input.taskId !== undefined ? reference(state.tasks, input.taskId) : (existing ? existing.taskId || null : null),
        documentIds: input.documentIds !== undefined ? references(state.documents, input.documentIds) : (existing ? existing.documentIds || [] : [])
      })
    },
    artifactStage (state, input) {
      let artifact = null
      if (input.id) {
        artifact = find(state.artifacts, input.id)
      } else if (input.externalId && input.source) {
        artifact = state.artifacts.find(row => row.externalId === input.externalId && row.source === input.source)
        if (!artifact) throw new Error('External artifact not registered')
      } else {
        throw new Error('Artifact not found')
      }
      artifact.stage = stage(input.stage)
      if (!artifact.stage) throw new Error('Invalid pipeline stage')
      if (input.toolUri) artifact.toolUri = uri(input.toolUri)
      if (input.vcsRevision) artifact.lastVcsRevision = text(input.vcsRevision, 200)
      artifact.stageNote = input.note ? text(input.note, 2000) : ''
      artifact.stageUpdatedAt = new Date().toISOString()
    },
    artifactVersion (state, input) {
      let artifactId = input.artifactId
      if (!artifactId && input.externalId) {
        const matched = state.artifacts.find(row => row.externalId === input.externalId && row.source === input.source)
        if (!matched) throw new Error('External artifact not registered')
        artifactId = matched.id
      }
      const parent = find(state.artifacts, artifactId)
      const version = text(input.version, 120)
      if (!/^[a-f0-9]{64}$/i.test(input.checksum)) throw new Error('Expected SHA-256 checksum')
      const resolvedStage = input.stage ? stage(input.stage) : (parent.stage || 'concept')
      const data = {
        artifactId,
        version,
        uri: uri(input.uri),
        checksum: input.checksum.toLowerCase(),
        previewUri: input.previewUri ? uri(input.previewUri) : null,
        toolUri: input.toolUri ? uri(input.toolUri) : null,
        stage: resolvedStage,
        vcsRevision: input.vcsRevision ? text(input.vcsRevision, 200) : null,
        vcsBranch: input.vcsBranch ? text(input.vcsBranch, 200) : null
      }
      const existing = state.artifactVersions.find(row => row.artifactId === artifactId && row.version === version)
      if (existing) {
        if (existing.checksum !== data.checksum || existing.uri !== data.uri || existing.previewUri !== data.previewUri) throw new Error('Artifact version already exists with different content')
        return
      }
      if (input.stage) parent.stage = resolvedStage
      if (input.toolUri && !parent.toolUri) parent.toolUri = data.toolUri
      if (data.vcsRevision) parent.lastVcsRevision = data.vcsRevision
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
    platformTarget (state, input) {
      const existing = input.id ? find(state.platformTargets, input.id) : null
      const sf = input.storefront || (existing && existing.storefront)
      if (!storefronts.includes(sf)) throw new Error('Invalid storefront platform')
      const subStatus = input.submissionStatus || (existing ? existing.submissionStatus : 'preparing')
      if (!submissionStatuses.includes(subStatus)) throw new Error('Invalid submission status')
      const preset = !existing ? buildPreset(sf, id, input.locale || 'pt') : null
      const values = {
        storefront: sf,
        name: input.name ? text(input.name, 200) : (existing ? existing.name : preset.defaultName),
        releaseId: input.releaseId !== undefined ? reference(state.releases, input.releaseId) : (existing ? existing.releaseId || null : null),
        storeUrl: input.storeUrl ? uri(input.storeUrl) : (input.storeUrl === '' ? '' : (existing ? existing.storeUrl || '' : '')),
        appIdentifier: input.appIdentifier ? text(input.appIdentifier, 200) : (input.appIdentifier === '' ? '' : (existing ? existing.appIdentifier || '' : '')),
        submissionStatus: subStatus,
        targetDate: input.targetDate !== undefined ? date(input.targetDate) : (existing ? existing.targetDate || null : null),
        notes: input.notes ? text(input.notes, 10000) : (input.notes === '' ? '' : (existing ? existing.notes || '' : '')),
        archived: input.archived === true
      }
      if (existing) {
        Object.assign(existing, values)
      } else {
        const usePreset = input.applyPreset !== false
        state.platformTargets.push({
          id: id(),
          ...values,
          checklist: usePreset ? preset.checklist : [],
          assets: usePreset ? preset.assets : [],
          createdAt: new Date().toISOString()
        })
      }
    },
    platformCheckItem (state, input, actor) {
      const target = find(state.platformTargets, input.targetId)
      const existing = input.id ? find(target.checklist, input.id) : null
      const category = input.category || (existing && existing.category)
      if (!checkCategories.includes(category)) throw new Error('Invalid checklist category')
      const status = input.status || (existing ? existing.status : 'pending')
      if (!itemStatuses.includes(status)) throw new Error('Invalid checklist status')
      const required = input.required !== undefined ? input.required !== false : (existing ? existing.required !== false : true)
      const waiverReason = input.waiverReason ? text(input.waiverReason, 2000) : (input.waiverReason === '' ? '' : (existing ? existing.waiverReason || '' : ''))
      if (status === 'na' && required && !waiverReason) {
        throw new Error('Specify waiver justification for N/A item')
      }
      const resolved = status === 'done' || status === 'na'
      const values = {
        category,
        title: input.title ? text(input.title, 300) : (existing ? existing.title : text(input.title, 300)),
        spec: input.spec ? text(input.spec, 2000) : (input.spec === '' ? '' : (existing ? existing.spec || '' : '')),
        required,
        status,
        waiverReason: status === 'na' ? waiverReason : '',
        assigneeId: input.assigneeId !== undefined ? reference(state.participants, input.assigneeId) : (existing ? existing.assigneeId || null : null),
        documentId: input.documentId !== undefined ? reference(state.documents, input.documentId) : (existing ? existing.documentId || null : null),
        taskId: input.taskId !== undefined ? reference(state.tasks, input.taskId) : (existing ? existing.taskId || null : null),
        notes: input.notes ? text(input.notes, 4000) : (input.notes === '' ? '' : (existing ? existing.notes || '' : '')),
        verifiedBy: resolved ? actor : null,
        verifiedAt: resolved ? new Date().toISOString() : null
      }
      save(target.checklist, input, values)
    },
    platformAssetSlot (state, input, actor) {
      const target = find(state.platformTargets, input.targetId)
      const existing = input.id ? find(target.assets, input.id) : null
      const status = input.status || (existing ? existing.status : 'pending')
      if (!itemStatuses.includes(status)) throw new Error('Invalid storefront asset status')
      const required = input.required !== undefined ? input.required !== false : (existing ? existing.required !== false : true)
      const waiverReason = input.waiverReason ? text(input.waiverReason, 2000) : (input.waiverReason === '' ? '' : (existing ? existing.waiverReason || '' : ''))
      if (status === 'na' && required && !waiverReason) {
        throw new Error('Specify waiver justification for N/A storefront asset')
      }
      const artifactVersionId = input.artifactVersionId !== undefined ?
        reference(state.artifactVersions, input.artifactVersionId) :
        (existing ? existing.artifactVersionId || null : null)
      const resolvedUri = input.uri ?
        uri(input.uri) :
        (input.uri === '' ? null : (existing ? existing.uri || null : null))
      const resolvedToolUri = input.toolUri ?
        uri(input.toolUri) :
        (input.toolUri === '' ? null : (existing ? existing.toolUri || null : null))
      if (status === 'done' && !artifactVersionId && !resolvedUri) {
        throw new Error('Attach an artifact version or file URI before marking storefront asset done')
      }
      let checksum = existing ? existing.checksum || null : null
      if (input.checksum !== undefined) {
        if (input.checksum === '' || input.checksum === null) checksum = null
        else if (!/^[a-f0-9]{64}$/i.test(input.checksum)) throw new Error('Expected SHA-256 checksum')
        else checksum = input.checksum.toLowerCase()
      }
      const resolved = status === 'done' || status === 'na'
      const values = {
        name: input.name ? text(input.name, 200) : (existing ? existing.name : text(input.name, 200)),
        dimensions: input.dimensions ? text(input.dimensions, 120) : (input.dimensions === '' ? '' : (existing ? existing.dimensions || '' : '')),
        format: input.format ? text(input.format, 120) : (input.format === '' ? '' : (existing ? existing.format || '' : '')),
        spec: input.spec ? text(input.spec, 2000) : (input.spec === '' ? '' : (existing ? existing.spec || '' : '')),
        required,
        status,
        waiverReason: status === 'na' ? waiverReason : '',
        artifactVersionId,
        uri: resolvedUri,
        checksum,
        toolUri: resolvedToolUri,
        locale: input.locale ? text(input.locale, 20) : (existing ? existing.locale || 'all' : 'all'),
        notes: input.notes ? text(input.notes, 4000) : (input.notes === '' ? '' : (existing ? existing.notes || '' : '')),
        verifiedBy: resolved ? actor : null,
        verifiedAt: resolved ? new Date().toISOString() : null
      }
      save(target.assets, input, values)
    },
    release (state, input) {
      const existing = input.id ? find(state.releases, input.id) : null
      if (existing && existing.publishedAt) throw new Error('Published release is immutable')
      const platformTargetIds = input.platformTargetIds !== undefined ?
        references(state.platformTargets, input.platformTargetIds) :
        (existing ? existing.platformTargetIds || [] : [])
      save(state.releases, input, { name: text(input.name), criteria: input.criteria ? text(input.criteria, 10000) : '', artifactVersionIds: references(state.artifactVersions, input.artifactVersionIds), validationIds: references(state.validations, input.validationIds), platformTargetIds, publishedAt: null })
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
      const explicitIds = new Set(release.platformTargetIds || [])
      const linkedTargets = state.platformTargets.filter(target => !target.archived && (target.releaseId === release.id || explicitIds.has(target.id)))
      for (const target of linkedTargets) {
        const evalResult = evaluateTargetReadiness(target)
        if (evalResult.missingChecks.length) {
          throw new Error('Platform compliance checklist incomplete: ' + target.name + ' (' + evalResult.missingChecks[0].title + ')')
        }
        if (evalResult.missingAssets.length) {
          throw new Error('Storefront assets incomplete: ' + target.name + ' (' + evalResult.missingAssets[0].name + ')')
        }
      }
      release.publishedAt = new Date().toISOString()
      release.publishedBy = actor
      release.buildIds = builds.map(build => build.id)
      release.platformTargetIds = linkedTargets.map(target => target.id)
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
      const values = {
        title: text(input.title),
        criteria: input.criteria ? text(input.criteria, 10000) : '',
        discipline: input.discipline ? text(input.discipline, 100) : '',
        phase: phase(input.phase, existing ? existing.phase || '' : ''),
        stage: stage(input.stage, existing ? existing.stage || '' : ''),
        toolUri: input.toolUri ? uri(input.toolUri) : (input.toolUri === '' ? null : (existing ? existing.toolUri || null : null)),
        assigneeId,
        priority,
        type,
        dueDate: date(input.dueDate),
        blockedReason: input.blockedReason ? text(input.blockedReason, 2000) : '',
        dependencies,
        milestoneId: reference(state.milestones, input.milestoneId),
        cycleId: reference(state.cycles, input.cycleId),
        documentIds: references(state.documents, input.documentIds)
      }
      if (existing) Object.assign(existing, values)
      else state.tasks.push({ id: taskId, ...values, status: 'backlog' })
    }
  }
  commands.integrationEvent = (state, input, actor) => {
    const source = text(input.source, 50)
    if (!tools.includes(source)) throw new Error('Unsupported source')
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
    if (!['artifact', 'artifactVersion', 'artifactStage', 'build', 'validation', 'vcsStatus'].includes(input.type)) throw new Error('Unsupported event type')
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
