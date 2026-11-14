const { createState, applyCommand } = require('../helpers/studio')

describe('studio production lifecycle', () => {
  let state
  const actor = 8
  const command = (name, input) => applyCommand(state, name, input, actor)
  beforeEach(() => { state = createState('Game', 'analogue', actor) })
  test('supports real teams and legacy participants', () => {
    command('team', { name: 'Design' })
    command('participant', { name: 'Designer', roles: ['Design'], teamId: state.teams[0].id })
    expect(state.participants[0].team).toBe('Design')
    command('team', { id: state.teams[0].id, name: 'Design', archived: true })
    expect(() => command('participant', { name: 'New', roles: ['Art'], teamId: state.teams[0].id })).toThrow('archived')
  })
  test('rejects dependency cycles and completing blocked work', () => {
    command('task', { title: 'A' })
    command('task', { title: 'B', dependencies: [state.tasks[0].id] })
    expect(() => command('task', { id: state.tasks[0].id, title: 'A', dependencies: [state.tasks[1].id] })).toThrow('cycle')
    expect(() => command('transition', { id: state.tasks[1].id, status: 'done' })).toThrow('blockers')
    command('transition', { id: state.tasks[0].id, status: 'done' })
    command('transition', { id: state.tasks[1].id, status: 'done' })
    expect(state.tasks[1].status).toBe('done')
  })
  test('planning references and cycle ranges are checked', () => {
    expect(() => command('cycle', { name: 'Sprint', startDate: '2026-12-10', endDate: '2026-12-01' })).toThrow()
    command('cycle', { name: 'Sprint', capacity: 12 })
    command('milestone', { name: 'Prototype', criteria: 'Playable' })
    command('task', { title: 'Cards', cycleId: state.cycles[0].id, milestoneId: state.milestones[0].id })
    expect(state.tasks[0].cycleId).toBe(state.cycles[0].id)
    expect(() => command('task', { title: 'Invalid', cycleId: 'missing' })).toThrow()
  })
  test('artifact versions cannot be overwritten', () => {
    command('artifact', { name: 'Cards', kind: 'print' })
    const input = { artifactId: state.artifacts[0].id, version: '1', uri: 'https://example.org/cards.pdf', checksum: 'a'.repeat(64) }
    command('artifactVersion', input)
    command('artifactVersion', input)
    expect(state.artifactVersions).toHaveLength(1)
    expect(() => command('artifactVersion', { ...input, checksum: 'b'.repeat(64) })).toThrow('different content')
    expect(() => command('artifactVersion', { ...input, version: '2', uri: 'javascript:alert(1)' })).toThrow()
  })
  test('integration retries are idempotent and reject altered event IDs', () => {
    const event = { source: 'geppetto', eventId: 'export-1', type: 'artifact', payload: { name: 'Portrait', kind: 'art', externalId: 'portrait' } }
    command('integrationEvent', event)
    command('integrationEvent', event)
    expect(state.artifacts).toHaveLength(1)
    expect(state.integrationEvents).toHaveLength(1)
    expect(() => command('integrationEvent', { ...event, payload: { ...event.payload, name: 'Other' } })).toThrow('reused')
    command('integrationEvent', { source: 'geppetto', eventId: 'export-2', type: 'artifactVersion', payload: { externalId: 'portrait', version: '1', uri: 'file:///C:/art/portrait.png', checksum: 'a'.repeat(64) } })
    expect(state.artifactVersions[0].artifactId).toBe(state.artifacts[0].id)
  })
  test('publishes only approved versions with passing evidence and credits', () => {
    command('participant', { name: 'Designer', roles: ['Design'] })
    command('artifact', { name: 'Cards', kind: 'print' })
    command('artifactVersion', { artifactId: state.artifacts[0].id, version: '1', uri: 'https://example.org/cards.pdf', checksum: 'a'.repeat(64) })
    const versionId = state.artifactVersions[0].id
    command('release', { name: 'Prototype', criteria: 'Playable', artifactVersionIds: [versionId] })
    const releaseId = state.releases[0].id
    expect(() => command('publishRelease', { id: releaseId })).toThrow('build')
    command('generateCredits', { releaseId, participantIds: [state.participants[0].id] })
    command('approveCredits', { id: state.credits[0].id })
    command('build', { name: 'Prototype 1', releaseId, snapshotId: state.credits[0].id })
    command('validation', { name: 'Playtest', kind: 'playtest', result: 'passed', releaseId, artifactVersionId: versionId })
    command('release', { id: releaseId, name: 'Prototype', criteria: 'Playable', artifactVersionIds: [versionId], validationIds: [state.validations[0].id] })
    expect(() => command('publishRelease', { id: releaseId })).toThrow('approval')
    command('reviewArtifact', { versionId, decision: 'approved' })
    command('publishRelease', { id: releaseId })
    expect(state.releases[0].publishedAt).toBeTruthy()
    expect(() => command('release', { id: releaseId, name: 'Changed' })).toThrow('immutable')
  })
  test('models macro phases and non-linear asset pipeline transitions with studio URIs', () => {
    expect(state.phase).toBe('pre_production')
    command('project', { name: 'GhostLore', kind: 'digital', phase: 'production', archived: false })
    expect(state.phase).toBe('production')
    expect(() => command('project', { name: 'GhostLore', kind: 'digital', phase: 'waterfall' })).toThrow('phase')

    command('milestone', { name: 'Vertical Slice', phase: 'production' })
    command('task', {
      title: 'Intro Dialogue',
      phase: 'production',
      stage: 'authoring',
      toolUri: 'goethe://open?repo=GhostLore&path=dialogues%2Fintro.yaml&node=start'
    })
    expect(state.tasks[0].toolUri).toContain('goethe://')

    command('artifact', {
      name: 'Intro Dialogue Graph',
      kind: 'dialogue',
      track: 'narrative',
      stage: 'concept',
      tool: 'goethe',
      assetPath: 'dialogues/intro.yaml',
      toolUri: 'goethe://open?repo=GhostLore&path=dialogues%2Fintro.yaml',
      taskId: state.tasks[0].id
    })
    const artifactId = state.artifacts[0].id
    command('artifactStage', { id: artifactId, stage: 'engine_integration', vcsRevision: 'abc12345' })
    expect(state.artifacts[0].stage).toBe('engine_integration')
    // Non-linear backward transition after in-engine director review:
    command('artifactStage', { id: artifactId, stage: 'blockout', note: 'Adjust pacing in previs' })
    expect(state.artifacts[0].stage).toBe('blockout')
    expect(state.artifacts[0].stageNote).toBe('Adjust pacing in previs')

    // Custom protocol allowlist vs credential / script URI rejection
    command('artifactVersion', {
      artifactId,
      version: 'v1',
      uri: 'lores://lore.roguefairy.net:41337/ghostlore/dialogues/intro.gdkg',
      toolUri: 'godot://open?project=GhostLore&scene=res%3A%2F%2Fscenes%2Fintro.tscn',
      stage: 'polish',
      vcsBranch: 'main',
      vcsRevision: 'main@14',
      checksum: 'c'.repeat(64)
    })
    expect(state.artifacts[0].stage).toBe('polish')
    expect(() => command('artifact', { name: 'Bad', kind: 'art', toolUri: 'goethe://user:secret@open?path=a' })).toThrow('credentials')
    expect(() => command('artifact', { name: 'Bad', kind: 'art', toolUri: 'data:text/html,1' })).toThrow('Unsupported')
  })
  test('tracks Git and Lore (Epic URC) repositories, working tree status, and binary file locks', () => {
    command('repository', {
      name: 'GhostLore Git',
      slug: 'ghostlore-code',
      system: 'git',
      remoteUri: 'git@github.com:RogueFairyStudios/GhostLore.git',
      defaultBranch: 'main',
      workspacePath: 'GhostLore'
    })
    expect(() => command('repository', {
      name: 'Duplicate',
      slug: 'ghostlore-code',
      system: 'git'
    })).toThrow('slug')
    expect(() => command('repository', {
      name: 'Leaked Creds',
      slug: 'leaked',
      system: 'lore',
      remoteUri: 'lores://token:secret@lore.roguefairy.net:41337/art'
    })).toThrow('credentials')

    // Auto-upsert Lore (Epic URC) repository and locks via integrationEvent
    command('integrationEvent', {
      source: 'lore',
      eventId: 'lore-sync-1',
      type: 'vcsStatus',
      payload: {
        slug: 'ghostlore-art',
        name: 'GhostLore Art (Lore)',
        system: 'lore',
        remoteUri: 'lores://lore.roguefairy.net:41337/ghostlore-art',
        branch: 'main',
        revision: 'f'.repeat(64),
        revisionNumber: 42,
        commitMessage: 'Lock & sculpt spirit bust',
        author: 'artist@roguefairy.net',
        modifiedFiles: ['Content/Characters/Spirit.uasset'],
        locks: [
          {
            path: 'Content/Characters/Spirit.uasset',
            owner: 'artist@roguefairy.net',
            branch: 'main',
            lockedAt: '2026-11-13T12:00:00Z'
          }
        ],
        compression: 'zstd'
      }
    })
    const loreRepo = state.repositories.find(r => r.slug === 'ghostlore-art')
    expect(loreRepo).toBeTruthy()
    expect(loreRepo.status.clean).toBe(false)
    expect(loreRepo.status.revisionNumber).toBe(42)
    expect(loreRepo.status.locks).toHaveLength(1)
    expect(loreRepo.status.locks[0].owner).toBe('artist@roguefairy.net')
    expect(() => command('vcsStatus', {
      repositoryId: loreRepo.id,
      modifiedFiles: ['../../etc/passwd']
    })).toThrow('Invalid asset path')
  })
  test('configures per-project Lore <project>/<repo> namespaces and validates restricted visibility', () => {
    command('loreConfig', {
      projectSlug: 'ghostlore',
      visibility: 'internal',
      employeeRole: 'editor',
      remoteBase: 'lores://lore.roguefairy.net',
      bridgeUrl: 'https://lore-auth.roguefairy.net',
      authUrl: 'https://auth.telluriancorp.info'
    })
    expect(state.lore.projectSlug).toBe('ghostlore')
    expect(state.lore.employeeRole).toBe('editor')
    expect(() => command('loreConfig', { bridgeUrl: 'http://insecure.roguefairy.net' })).toThrow('HTTPS')

    command('repository', {
      name: 'GhostLore Client',
      system: 'lore',
      loreRepo: 'client',
      workspacePath: 'games/ghostlore'
    })
    const clientRepo = state.repositories.find(r => r.loreRepo === 'client')
    expect(clientRepo.slug).toBe('ghostlore-client')
    expect(clientRepo.loreFullName).toBe('ghostlore/client')
    expect(clientRepo.remoteUri).toBe('lores://lore.roguefairy.net/ghostlore/client')

    expect(() => command('repository', {
      name: 'Missing Namespace',
      slug: 'bad-lore',
      system: 'lore',
      remoteUri: 'lores://lore.roguefairy.net/unscoped'
    })).toThrow('<project>/<repo>')
    expect(() => command('repository', {
      name: 'Traversal',
      slug: 'bad-seg',
      system: 'lore',
      loreRepo: '../escape'
    })).toThrow('Invalid Lore repository name')

    command('vcsStatus', {
      slug: 'ghostlore-cinematics',
      system: 'lore',
      remoteUri: 'lores://lore.roguefairy.net/ghostlore/cinematics',
      branch: 'main',
      revisionNumber: 7
    })
    const cinematics = state.repositories.find(r => r.slug === 'ghostlore-cinematics')
    expect(cinematics.loreFullName).toBe('ghostlore/cinematics')

    expect(() => command('loreConfig', { visibility: 'restricted', employeeRole: 'reader' })).toThrow('Restricted')
    command('loreConfig', { visibility: 'restricted' })
    expect(state.lore.visibility).toBe('restricted')
    expect(state.lore.employeeRole).toBe('none')
    expect(() => command('loreConfig', { visibility: 'internal' })).toThrow('Confirm')
    command('loreConfig', { visibility: 'internal', confirmUnrestrict: true })
    expect(state.lore.visibility).toBe('internal')
  })
  test('populates storefront presets and gates release publication on compliance and store assets', () => {
    const majorStores = ['steam', 'epic', 'apple_app_store', 'google_play', 'playstation', 'xbox', 'nintendo', 'igdb', 'bgg_tabletop']
    for (const sf of majorStores) {
      command('platformTarget', { storefront: sf, locale: 'pt' })
    }
    expect(state.platformTargets).toHaveLength(majorStores.length)
    const steam = state.platformTargets.find(t => t.storefront === 'steam')
    expect(steam.checklist.some(c => c.category === 'technical' && c.title.includes('SteamPipe'))).toBe(true)
    expect(steam.checklist.some(c => c.category === 'bureaucratic' && c.title.includes('ClassInd'))).toBe(true)
    expect(steam.assets.some(a => a.name === 'Header Capsule' && a.dimensions === '920×430 px')).toBe(true)
    expect(steam.assets.some(a => a.name === 'Library Hero' && a.dimensions === '3840×1240 px')).toBe(true)

    const igdb = state.platformTargets.find(t => t.storefront === 'igdb')
    expect(igdb.assets.some(a => a.dimensions.includes('1080×1440'))).toBe(true)

    // Setup a release linked to IGDB target to test publication gating
    command('participant', { name: 'Producer', roles: ['Production'] })
    command('artifact', { name: 'Master Build', kind: 'binary' })
    command('artifactVersion', {
      artifactId: state.artifacts[0].id,
      version: '1.0.0',
      uri: 'https://docs.roguefairy.net/p/studio/files/pkg.zip',
      checksum: 'd'.repeat(64)
    })
    const versionId = state.artifactVersions[0].id
    command('reviewArtifact', { versionId, decision: 'approved' })
    command('release', {
      name: '1.0 Launch',
      criteria: 'Gold Master',
      artifactVersionIds: [versionId],
      platformTargetIds: [igdb.id]
    })
    const releaseId = state.releases[0].id
    command('generateCredits', { releaseId, participantIds: [state.participants[0].id] })
    command('approveCredits', { id: state.credits[0].id })
    command('build', { name: 'GM-1', releaseId, snapshotId: state.credits[0].id })
    command('validation', { name: 'QA Cert', kind: 'test', result: 'passed', releaseId, artifactVersionId: versionId })
    command('release', {
      id: releaseId,
      name: '1.0 Launch',
      criteria: 'Gold Master',
      artifactVersionIds: [versionId],
      validationIds: [state.validations[0].id],
      platformTargetIds: [igdb.id]
    })

    // Blocked while IGDB technical/bureaucratic checks are pending
    expect(() => command('publishRelease', { id: releaseId })).toThrow('Platform compliance checklist incomplete')

    // N/A without justification must fail
    expect(() => command('platformCheckItem', {
      targetId: igdb.id,
      id: igdb.checklist[0].id,
      status: 'na'
    })).toThrow('waiver justification')

    // Complete or waive all IGDB checks
    command('platformCheckItem', {
      targetId: igdb.id,
      id: igdb.checklist[0].id,
      status: 'na',
      waiverReason: 'Single-store launch initially'
    })
    for (const chk of igdb.checklist.slice(1)) {
      command('platformCheckItem', { targetId: igdb.id, id: chk.id, status: 'done' })
    }

    // Still blocked because IGDB storefront assets are pending
    expect(() => command('publishRelease', { id: releaseId })).toThrow('Storefront assets incomplete')

    // Marking an asset slot done without URI or artifactVersionId must fail
    expect(() => command('platformAssetSlot', {
      targetId: igdb.id,
      id: igdb.assets[0].id,
      status: 'done'
    })).toThrow('Attach an artifact version or file URI')

    // Provide storefront assets (including Geppetto source toolUri)
    for (const slot of igdb.assets) {
      command('platformAssetSlot', {
        targetId: igdb.id,
        id: slot.id,
        status: 'done',
        uri: 'https://docs.roguefairy.net/p/studio/files/' + slot.id + '.png',
        toolUri: 'geppetto://open?repo=GhostLore&path=marketing%2Figdb_cover.kra',
        checksum: 'e'.repeat(64)
      })
    }

    // Now release publishes cleanly and snapshots platformTargetIds
    command('publishRelease', { id: releaseId })
    expect(state.releases[0].publishedAt).toBeTruthy()
    expect(state.releases[0].platformTargetIds).toEqual([igdb.id])
  })
})
