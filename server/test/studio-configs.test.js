/** @jest-environment node */
const http = require('http')
const express = require('express')
const yaml = require('js-yaml')
const router = require('../controllers/studio')
const { createState, applyCommand } = require('../helpers/studio')

describe('studio generalist and project-specific tool configuration endpoints', () => {
  let server
  let projectsById
  let usersById
  let groupsById

  beforeEach(async () => {
    delete process.env.STUDIO_API_URL
    delete process.env.STUDIO_INTERNAL_KEY

    groupsById = {
      10: { id: 10, name: 'Rogue Fairy Studios - C-Level' },
      11: { id: 11, name: 'Rogue Fairy Studios' },
      12: { id: 12, name: 'Rogue Fairy Studios - Partners' },
      25: { id: 25, name: 'Klaus Syndicate - Artists & Animators' },
      26: { id: 26, name: 'Klaus Syndicate - Secret Leads' }
    }

    usersById = {
      1: { id: 1, name: 'Founder Admin', email: 'founder@roguefairy.net', isActive: true, groups: [10] },
      2: { id: 2, name: 'Guest', email: 'guest@example.com', isActive: true, groups: [] },
      8: { id: 8, name: 'Clara Dev', email: 'clara@roguefairy.net', isActive: true, groups: [11] },
      3: { id: 3, name: 'Mateus Animator', email: 'mateus@roguefairy.net', isActive: true, groups: [11, 25] },
      4: { id: 4, name: 'External Partner', email: 'partner@external.example', isActive: true, groups: [12] }
    }

    const klausState = createState('Klaus Syndicate', 'digital', 1, 'aetherion')
    applyCommand(klausState, 'loreConfig', {
      projectSlug: 'klaus-syndicate',
      visibility: 'internal',
      employeeRole: 'editor',
      remoteBase: 'lores://lore.roguefairy.net',
      bridgeUrl: 'https://lore-auth.roguefairy.net',
      authUrl: 'https://auth.telluriancorp.info'
    }, 1)
    klausState.groupAccess.push(
      {
        id: 'ga-25',
        groupId: 25,
        groupName: 'Klaus Syndicate - Artists & Animators',
        role: 'editor',
        loreActions: ['read', 'write', 'presign'],
        loreCanCreateRepo: true
      },
      {
        id: 'ga-26',
        groupId: 26,
        groupName: 'Klaus Syndicate - Secret Leads',
        role: 'manager',
        loreActions: ['read', 'write', 'admin'],
        loreCanCreateRepo: true
      }
    )
    applyCommand(klausState, 'repository', {
      name: 'Klaus Syndicate Engine',
      slug: 'klaus-engine',
      system: 'git',
      remoteUri: 'git@github.com:RogueFairyStudios/klaus-syndicate.git',
      defaultBranch: 'main',
      workspacePath: 'games/klaus-syndicate/project.godot'
    }, 1)
    applyCommand(klausState, 'repository', {
      name: 'Klaus Syndicate Art',
      slug: 'klaus-art',
      system: 'lore',
      loreRepo: 'art',
      defaultBranch: 'main',
      workspacePath: 'art/klaus-syndicate'
    }, 1)
    applyCommand(klausState, 'repository', {
      name: 'Klaus Syndicate Animations',
      slug: 'klaus-animations',
      system: 'lore',
      loreRepo: 'animations',
      defaultBranch: 'main',
      workspacePath: 'art/klaus-syndicate-anim'
    }, 1)
    const artRepo = klausState.repositories.find(r => r.slug === 'klaus-art')
    applyCommand(klausState, 'vcsStatus', {
      repositoryId: artRepo.id,
      branch: 'main',
      revision: 'main@18',
      revisionNumber: 18,
      clean: false,
      modifiedCount: 1,
      locks: [{ path: 'Characters/Klaus/klaus_idle.kra', owner: 'mateus@roguefairy.net', branch: 'main' }]
    }, 1)
    applyCommand(klausState, 'artifact', {
      name: 'Klaus Idle Animation (.kra)',
      kind: 'animation',
      track: 'art_2d',
      stage: 'authoring',
      tool: 'geppetto',
      repoId: artRepo.id,
      assetPath: 'Characters/Klaus/klaus_idle.kra',
      toolUri: 'geppetto://open?repo=art%2Fklaus-syndicate&path=Characters%2FKlaus%2Fklaus_idle.kra'
    }, 1)
    applyCommand(klausState, 'platformTarget', {
      storefront: 'steam',
      name: 'Steam',
      applyPreset: true
    }, 1)

    projectsById = {
      'proj-klaus-uuid': {
        id: 'proj-klaus-uuid',
        revision: 10,
        state: JSON.stringify(klausState)
      }
    }

    global.WIKI = {
      config: { host: 'https://docs.roguefairy.net' },
      auth: {
        groups: groupsById,
        checkAccess: (user, perms) => {
          if (!user || !user.id) return false
          if (perms.includes('manage:system')) return user.id === 1
          if (perms.includes('read:pages') || perms.includes('write:pages')) return true
          return false
        }
      },
      models: {
        users: {
          query: () => ({
            findById: async id => usersById[id] || null
          })
        },
        studioProjects: {
          query: () => ({
            findById: async id => projectsById[id] || null,
            then: (resolve, reject) => Promise.resolve(Object.values(projectsById)).then(resolve, reject)
          })
        }
      }
    }

    const app = express()
    app.use((req, res, next) => {
      const uid = Number(req.headers['x-test-user'] || 0)
      if (uid && usersById[uid]) {
        req.user = { id: uid, email: usersById[uid].email, name: usersById[uid].name, groups: usersById[uid].groups }
      }
      next()
    })
    app.use(router)
    server = app.listen(0, '127.0.0.1')
    await new Promise(resolve => server.once('listening', resolve))
  })

  afterEach(async () => {
    if (server) {
      await new Promise(resolve => server.close(resolve))
    }
    delete global.WIKI
  })

  function request (userId, reqPath, headers = {}) {
    return new Promise((resolve, reject) => {
      const mergedHeaders = { ...headers }
      if (userId) mergedHeaders['x-test-user'] = String(userId)
      const req = http.request({
        host: '127.0.0.1',
        port: server.address().port,
        method: 'GET',
        path: reqPath,
        headers: mergedHeaders
      }, res => {
        const chunks = []
        res.on('data', chunk => chunks.push(chunk))
        res.on('end', () => resolve({
          status: res.statusCode,
          headers: res.headers,
          body: Buffer.concat(chunks).toString('utf8')
        }))
      })
      req.on('error', reject)
      req.end()
    })
  }

  test('serves generalist studio configurations in JSON and YAML for Aetherion, Geppetto and Authentik', async () => {
    const catalogRes = await request(8, '/p/studio/configs')
    expect(catalogRes.status).toBe(200)
    const catalog = JSON.parse(catalogRes.body)
    expect(catalog.schemaVersion).toBe('rfs.studio.config/v1')
    expect(catalog.scope).toBe('studio')
    expect(catalog.profiles.map(t => t.tool)).toEqual(expect.arrayContaining(['aetherion', 'geppetto', 'authentik', 'lore', 'goethe', 'all']))

    const aetherionRes = await request(8, '/p/studio/configs/aetherion')
    expect(aetherionRes.status).toBe(200)
    const aetherionCfg = JSON.parse(aetherionRes.body)
    expect(aetherionCfg.scope).toBe('studio')
    expect(aetherionCfg.tool).toBe('aetherion')
    expect(aetherionCfg.authentik.baseUrl).toBe('https://auth.telluriancorp.info')
    expect(aetherionCfg.editorSettings['network/authentik/issuer_url']).toBe('https://auth.telluriancorp.info/application/o/aetherion')

    const geppettoYamlRes = await request(3, '/p/studio/configs/geppetto.yaml?download=1')
    expect(geppettoYamlRes.status).toBe(200)
    expect(geppettoYamlRes.headers['content-type']).toContain('application/yaml')
    expect(geppettoYamlRes.headers['content-disposition']).toContain('roguefairy-geppetto.yaml')
    expect(geppettoYamlRes.body).toContain('# Rogue Fairy Studios — Generalist Tool Configuration (geppetto)')
    const geppettoCfg = yaml.safeLoad(geppettoYamlRes.body)
    expect(geppettoCfg.tool).toBe('geppetto')
    expect(geppettoCfg.authentik.baseUrl).toBe('https://auth.telluriancorp.info')
    expect(geppettoCfg.lore.remoteBase).toBe('lores://lore.roguefairy.net')
    expect(geppettoCfg.kritaVersioningSettings.configGroup).toBe('Versioning')
  })

  test('serves Klaus Syndicate developer (Aetherion) and artist/animator (Geppetto) configs in JSON and YAML', async () => {
    // 1. Developer requesting Klaus Syndicate Aetherion workstation config in YAML by slug
    const devRes = await request(8, '/p/studio/projects/klaus-syndicate/configs/aetherion?format=yaml')
    expect(devRes.status).toBe(200)
    const devCfg = yaml.safeLoad(devRes.body)
    expect(devCfg.scope).toBe('project')
    expect(devCfg.tool).toBe('aetherion')
    expect(devCfg.project.name).toBe('Klaus Syndicate')
    expect(devCfg.project.slug).toBe('klaus-syndicate')
    expect(devCfg.project.engine).toBe('aetherion')
    expect(devCfg.project.usesAetherion).toBe(true)
    expect(devCfg.projectSettings['application/config/name']).toBe('Klaus Syndicate')
    expect(devCfg.workspace.primaryWorkspacePath).toBe('games/klaus-syndicate/project.godot')
    expect(devCfg.repositories.filter(r => r.system === 'git')).toHaveLength(1)
    expect(devCfg.repositories.filter(r => r.system === 'lore')).toHaveLength(2)

    // 2. Artist/animator requesting Klaus Syndicate Geppetto config in JSON (also works via UUID)
    const artRes = await request(3, '/p/studio/projects/proj-klaus-uuid/configs/geppetto')
    expect(artRes.status).toBe(200)
    const artCfg = JSON.parse(artRes.body)
    expect(artCfg.scope).toBe('project')
    expect(artCfg.tool).toBe('geppetto')
    expect(artCfg.project.name).toBe('Klaus Syndicate')
    expect(artCfg.authentik.baseUrl).toBe('https://auth.telluriancorp.info')
    expect(artCfg.lore.remoteBase).toBe('lores://lore.roguefairy.net')
    expect(artCfg.lore.namespace).toBe('klaus-syndicate/*')
    expect(artCfg.kritaVersioningSettings.projectEntry.accessLevel).toBe('read-write')
    expect(artCfg.kritaVersioningSettings.projectEntry.canCreateRepo).toBe(true)
    expect(artCfg.kritaVersioningSettings.projectEntry.allowedActions).toEqual(expect.arrayContaining(['read', 'write', 'presign']))
    expect(artCfg.repositories.filter(r => r.system === 'lore').map(r => r.remoteUri)).toEqual(expect.arrayContaining([
      'lores://lore.roguefairy.net/klaus-syndicate/art',
      'lores://lore.roguefairy.net/klaus-syndicate/animations'
    ]))
    const lockedRepo = artCfg.repositories.find(r => r.slug === 'klaus-art')
    expect(lockedRepo.locks).toHaveLength(1)
    expect(lockedRepo.locks[0].path).toBe('Characters/Klaus/klaus_idle.kra')
    expect(lockedRepo.locks[0].ownedByUser).toBe(true)
    expect(artCfg.artPipeline.artifacts).toHaveLength(1)
    expect(artCfg.artPipeline.storefrontSlots.length).toBeGreaterThan(0)

    // Non-manager artist only sees matched groupGrants, never other secret groups
    expect(artCfg.access.groupGrants.map(g => g.groupName)).toEqual(['Klaus Syndicate - Artists & Animators'])
  })

  test('enforces authentication and fail-closed Partner isolation on configuration endpoints', async () => {
    const unauth = await request(0, '/p/studio/configs/aetherion')
    expect(unauth.status).toBe(403)

    const guestRes = await request(2, '/p/studio/configs/aetherion')
    expect(guestRes.status).toBe(403)

    const partnerProject = await request(4, '/p/studio/projects/klaus-syndicate/configs/geppetto')
    expect(partnerProject.status).toBe(404)
  })
})
