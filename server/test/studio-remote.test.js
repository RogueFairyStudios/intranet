const { spawn, execFileSync } = require('child_process')
const http = require('http')
const path = require('path')
const fs = require('fs')
const readline = require('readline')
const yaml = require('js-yaml')
const resolver = require('../graph/resolvers/studio')
const studioRemote = require('../helpers/studio-remote')

function ensureGoServerBinary () {
  const serverDir = path.resolve(__dirname, '..', '..', 'studio-server')
  const ext = process.platform === 'win32' ? '.exe' : ''
  const binPath = path.join(serverDir, 'bin', `studio-server${ext}`)
  if (!fs.existsSync(binPath)) {
    fs.mkdirSync(path.dirname(binPath), { recursive: true })
    execFileSync('go', ['build', '-o', binPath, './cmd/studio-server'], {
      cwd: serverDir,
      stdio: 'inherit'
    })
  }
  return { serverDir, binPath }
}

function getJSON (url, headers = {}) {
  return new Promise((resolve, reject) => {
    const req = http.get(url, { headers }, res => {
      const chunks = []
      res.on('data', c => chunks.push(c))
      res.on('end', () => {
        try {
          resolve({ status: res.statusCode, body: JSON.parse(Buffer.concat(chunks).toString('utf8')) })
        } catch (err) {
          reject(err)
        }
      })
    })
    req.on('error', reject)
  })
}

describe('Wiki.js <-> Go studio-server live delegation', () => {
  let child = null
  let baseUrl = ''
  let pageUpdatedAt = '2026-11-14T10:00:00.000Z'
  const internalKey = 'test-wiki-go-internal-secret'
  const ciServiceKey = 'stk_test_ci_integration_secret_0123456789'

  beforeAll(async () => {
    const { serverDir, binPath } = ensureGoServerBinary()
    await new Promise((resolve, reject) => {
      child = spawn(binPath, [], {
        cwd: serverDir,
        env: {
          ...process.env,
          STUDIO_DATABASE_URL: '',
          DATABASE_URL: '',
          STUDIO_ADDR: '127.0.0.1:0',
          STUDIO_PRINT_LISTENER: '1',
          STUDIO_INTERNAL_KEY: internalKey,
          STUDIO_SERVICE_KEYS: `ci:admin:${ciServiceKey}`
        },
        stdio: ['ignore', 'pipe', 'pipe']
      })
      const rl = readline.createInterface({ input: child.stdout })
      const timer = setTimeout(() => reject(new Error('Timed out waiting for studio-server startup')), 10000)
      rl.on('line', line => {
        const match = /^STUDIO_READY addr=(.+)$/.exec(line.trim())
        if (match) {
          clearTimeout(timer)
          baseUrl = `http://${match[1]}`
          resolve()
        }
      })
      child.once('exit', code => {
        clearTimeout(timer)
        reject(new Error(`studio-server exited early with code ${code}`))
      })
    })

    process.env.STUDIO_API_URL = baseUrl
    process.env.STUDIO_INTERNAL_KEY = internalKey
  })

  afterAll(async () => {
    delete process.env.STUDIO_API_URL
    delete process.env.STUDIO_INTERNAL_KEY
    delete global.WIKI
    if (child && !child.killed) {
      child.kill()
      await new Promise(resolve => child.once('exit', resolve))
    }
  })

  beforeEach(() => {
    const pagesById = {
      55: {
        id: 55,
        title: 'Abyssum Art Bible',
        path: 'games/abyssum/art-bible',
        localeCode: 'pt',
        updatedAt: pageUpdatedAt,
        isPublished: true,
        isPrivate: false,
        tags: []
      }
    }
    const usersById = {
      1: { id: 1, name: 'Founder Admin', email: 'founder@roguefairy.net', isActive: true },
      9: { id: 9, name: 'Lucas Engine', email: 'lucas@roguefairy.net', isActive: true },
      10: { id: 10, name: 'Bianca Art', email: 'bianca@roguefairy.net', isActive: true }
    }
    const groupsById = {
      20: { id: 20, name: 'Rogue Fairy Studios - C-Level' },
      21: { id: 21, name: 'Rogue Fairy Studios' },
      25: { id: 25, name: 'Rogue Fairy Studios - Abyssum - Art' }
    }
    global.WIKI = {
      config: { uploads: { maxFileSize: 10 * 1024 * 1024 } },
      auth: {
        groups: groupsById,
        checkAccess: (user, perms) => {
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
        groups: {
          query: () => ({
            findById: async id => groupsById[id] || null
          })
        },
        pages: {
          query: () => ({
            findById: id => ({
              withGraphFetched: async () => pagesById[id] ? { ...pagesById[id], updatedAt: pageUpdatedAt } : null
            }),
            findOne: ({ path: p }) => ({
              withGraphFetched: async () => Object.values(pagesById).find(pg => pg.path === p) || null
            })
          }),
          createPage: async ({ path: p, locale, title, tags }) => {
            const created = {
              id: 77,
              title,
              path: p,
              localeCode: locale,
              updatedAt: pageUpdatedAt,
              isPublished: true,
              isPrivate: false,
              tags: tags.map(t => ({ tag: t }))
            }
            pagesById[77] = created
            return created
          }
        }
      }
    }
  })

  const ctxFor = (id, email, name, groups) => ({
    req: {
      user: { id, email, name, groups, permissions: [] }
    }
  })

  test('delegates project creation, commands, documents, files, and exposes live cross-tool queries', async () => {
    expect(studioRemote.isEnabled()).toBe(true)

    const adminCtx = ctxFor(1, 'founder@roguefairy.net', 'Founder Admin', [20])
    const lucasCtx = ctxFor(9, 'lucas@roguefairy.net', 'Lucas Engine', [21])
    const biancaCtx = ctxFor(10, 'bianca@roguefairy.net', 'Bianca Art', [21, 25])

    // 1. Create project via Wiki.js GraphQL mutation -> stored in Go server
    const created = await resolver.Mutation.studioCreateProject(null, {
      name: 'Abyssum',
      kind: 'digital'
    }, adminCtx)
    expect(created.revision).toBe(1)
    expect(created.canManage).toBe(true)
    const projectId = created.id

    let rev = created.revision
    const exec = async (cmd, input, ctx = adminCtx) => {
      const out = await resolver.Mutation.studioCommand(null, {
        projectId,
        revision: rev,
        command: cmd,
        input: JSON.stringify(input)
      }, ctx)
      rev = out.revision
      return out
    }

    // 2. Configure Lore, repositories, group access, direct access, participants, tasks, and Wiki documents
    await exec('loreConfig', {
      projectSlug: 'abyssum',
      visibility: 'internal',
      employeeRole: 'editor'
    })
    await exec('groupAccess', {
      groupId: 25,
      role: 'editor',
      loreActions: ['read', 'write', 'presign'],
      loreCanCreateRepo: true
    })
    await exec('access', {
      userId: 9,
      role: 'editor'
    })
    await exec('repository', {
      name: 'Abyssum Engine',
      slug: 'abyssum-engine',
      system: 'git',
      remoteUri: 'https://github.com/RogueFairyStudios/abyssum.git',
      workspacePath: 'engine/project.godot'
    })
    await exec('repository', {
      name: 'Abyssum Lore Art',
      system: 'lore',
      loreRepo: 'art-assets'
    })
    await exec('participant', {
      name: 'Bianca Art',
      roles: ['Lead 2D Artist'],
      userId: 10
    })

    const afterDoc = await exec('document', { pageId: 55 }, biancaCtx)
    const docState = JSON.parse(afterDoc.state)
    expect(docState.documents).toHaveLength(1)
    expect(docState.documents[0].title).toBe('Abyssum Art Bible')
    expect(docState.documents[0].outdated).toBe(false)
    expect(docState.access).toEqual([]) // Non-manager has ACLs redacted

    // Update Wiki page timestamp and verify outdated flag + refreshDocument
    pageUpdatedAt = '2026-11-14T12:30:00.000Z'
    const staleView = await resolver.Query.studioProject(null, { id: projectId }, biancaCtx)
    expect(JSON.parse(staleView.state).documents[0].outdated).toBe(true)

    const refreshed = await exec('refreshDocument', {
      id: docState.documents[0].id
    }, biancaCtx)
    expect(JSON.parse(refreshed.state).documents[0].outdated).toBe(false)

    // 3. Binary file upload & download delegated through studioRemote
    const fileBuf = Buffer.from('KRITA_LAYER_PREVIEW_BYTES', 'utf8')
    const uploaded = await studioRemote.uploadFile(projectId, 'boss_concept.png', fileBuf, biancaCtx.req.user)
    expect(uploaded.id).toBeTruthy()
    const downloaded = await studioRemote.downloadFile(projectId, uploaded.id, lucasCtx.req.user)
    expect(downloaded.statusCode).toBe(200)
    expect(downloaded.filename).toBe('boss_concept.png')
    expect(downloaded.data.equals(fileBuf)).toBe(true)

    // 4. Query Go server REST API directly (simulating Aetherion & Geppetto)
    const aetherionView = await getJSON(`${baseUrl}/v1/me/projects?client=aetherion`, {
      'X-Studio-Internal-Key': internalKey,
      'X-Studio-Actor': studioRemote.encodeActor(lucasCtx.req.user)
    })
    expect(aetherionView.status).toBe(200)
    expect(aetherionView.body.clientApp).toBe('aetherion')
    expect(aetherionView.body.projects).toHaveLength(1)
    expect(aetherionView.body.projects[0].name).toBe('Abyssum')
    expect(aetherionView.body.projects[0].primaryWorkspacePath).toBe('engine/project.godot')

    const geppettoView = await getJSON(`${baseUrl}/v1/me/workspaces?client=geppetto&vcs=lore&onlyWritable=1`, {
      'X-Studio-Internal-Key': internalKey,
      'X-Studio-Actor': studioRemote.encodeActor(biancaCtx.req.user)
    })
    expect(geppettoView.status).toBe(200)
    expect(geppettoView.body.workspaces).toHaveLength(1)
    expect(geppettoView.body.workspaces[0].repositories[0].remoteUri).toBe('lores://lore.roguefairy.net/abyssum/art-assets')
    expect(geppettoView.body.workspaces[0].lore.canCreateRepo).toBe(true)

    // 5. Verify generalist & project-specific JSON/YAML config delegation to Go server
    const studioAetherionCfg = await studioRemote.fetchConfig('/v1/configs/aetherion', {
      user: {
        id: 9,
        email: 'lucas@roguefairy.net',
        name: 'Lucas Engine',
        isSystemAdmin: false,
        groups: [{ id: 21, name: 'Rogue Fairy Studios' }]
      }
    })
    expect(studioAetherionCfg.statusCode).toBe(200)
    const parsedAetherion = JSON.parse(studioAetherionCfg.body.toString('utf8'))
    expect(parsedAetherion.scope).toBe('studio')
    expect(parsedAetherion.tool).toBe('aetherion')
    expect(parsedAetherion.authentik.baseUrl).toBe('https://auth.telluriancorp.info')

    const projectGeppettoYaml = await studioRemote.fetchConfig('/v1/projects/abyssum/configs/geppetto?format=yaml&download=1', {
      user: {
        id: 10,
        email: 'bianca@roguefairy.net',
        name: 'Bianca Art',
        isSystemAdmin: false,
        groups: [
          { id: 21, name: 'Rogue Fairy Studios' },
          { id: 25, name: 'Rogue Fairy Studios - Abyssum - Art' }
        ]
      }
    })
    expect(projectGeppettoYaml.statusCode).toBe(200)
    expect(projectGeppettoYaml.contentDisposition).toContain('abyssum-geppetto.yaml')
    const parsedGeppettoYaml = yaml.safeLoad(projectGeppettoYaml.body.toString('utf8'))
    expect(parsedGeppettoYaml.scope).toBe('project')
    expect(parsedGeppettoYaml.tool).toBe('geppetto')
    expect(parsedGeppettoYaml.project.slug).toBe('abyssum')
    expect(parsedGeppettoYaml.kritaVersioningSettings.projectEntry.canCreateRepo).toBe(true)
    expect(parsedGeppettoYaml.repositories[0].remoteUri).toBe('lores://lore.roguefairy.net/abyssum/art-assets')
  })
})
