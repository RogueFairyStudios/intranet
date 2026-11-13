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
})
