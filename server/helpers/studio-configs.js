const yaml = require('js-yaml')
const studio = require('./studio')
const { roleFor } = require('./studio-access')

/* global WIKI */

const CONFIG_SCHEMA_VERSION = 'rfs.studio.config/v1'

const TOOL_CATALOG = [
  {
    tool: 'aetherion',
    title: 'Aetherion Engine',
    description: 'Engine configuration (Tellurian Authentik SSO, Studio API, Git/Lore workspace & project settings)'
  },
  {
    tool: 'geppetto',
    title: 'Geppetto (2D Art & Animation)',
    description: 'Artist & animator configuration (Tellurian Authentik, Lore VCS server, repositories, locks & access levels)'
  },
  {
    tool: 'authentik',
    title: 'Tellurian Authentik SSO',
    description: 'OIDC provider endpoints, PKCE loopback rules, studio groups & application slugs'
  },
  {
    tool: 'lore',
    title: 'Lore VCS (GhostLore)',
    description: 'Lore VCS server, OIDC bridge, namespace permissions & repository clone commands'
  },
  {
    tool: 'goethe',
    title: 'Goethe Narrative Studio',
    description: 'Dialogue & narrative workspace (.gproj.yaml template, .gdkg zstd archive & narrative assets)'
  },
  {
    tool: 'carnifex',
    title: 'Carnifex Build & Release',
    description: 'Build pipeline, release targets, approved credits snapshot & storefront gates'
  },
  {
    tool: 'animafaber',
    title: 'Animafaber',
    description: 'Character & spirit pipeline OIDC loopback (port 44200), Studio API & assets'
  },
  {
    tool: 'fontpit',
    title: 'Fontpit',
    description: 'Typography & font pipeline OIDC, Studio API & assets'
  },
  {
    tool: 'uri-handler',
    title: 'Rogue URI Handler',
    description: 'Workstation URI scheme dispatcher (geppetto://, goethe://, aetherion://, godot://, lore://)'
  },
  {
    tool: 'all',
    title: 'All Studio Tools Bundle',
    description: 'Complete composite workstation configuration across all Rogue Fairy Studios tools'
  }
]

function parseClientIdMap (raw) {
  const out = { 'rfs-intranet': 'intranet-rfs' }
  for (const part of String(raw || '').split(',')) {
    const trimmed = part.trim()
    if (!trimmed) continue
    const idx = trimmed.search(/[:=]/)
    if (idx <= 0 || idx === trimmed.length - 1) continue
    const slug = trimmed.slice(0, idx).trim().toLowerCase()
    const cid = trimmed.slice(idx + 1).trim()
    if (slug && cid) out[slug] = cid
  }
  return out
}

function resolveEnvOptions () {
  const intranetUrl = String(
    process.env.STUDIO_INTRANET_URL ||
    (typeof WIKI !== 'undefined' && WIKI.config && WIKI.config.host) ||
    'https://docs.roguefairy.net'
  ).trim().replace(/\/+$/, '')

  const studioApiUrl = String(
    process.env.STUDIO_PUBLIC_URL ||
    process.env.STUDIO_API_URL ||
    `${intranetUrl}/p/studio`
  ).trim().replace(/\/+$/, '')

  const oidcBaseUrl = String(process.env.STUDIO_OIDC_BASE_URL || studio.loreDefaults.authUrl).trim().replace(/\/+$/, '')
  const loreRemoteBase = String(process.env.STUDIO_LORE_REMOTE_BASE || studio.loreDefaults.remoteBase).trim().replace(/\/+$/, '')
  const loreBridgeUrl = String(process.env.STUDIO_LORE_BRIDGE_URL || studio.loreDefaults.bridgeUrl).trim().replace(/\/+$/, '')
  const loreAuthUrl = String(process.env.STUDIO_LORE_AUTH_URL || oidcBaseUrl).trim().replace(/\/+$/, '')
  const clientIds = parseClientIdMap(process.env.STUDIO_OIDC_CLIENT_IDS)

  return {
    intranetUrl,
    studioApiUrl,
    oidcBaseUrl,
    loreRemoteBase,
    loreBridgeUrl,
    loreAuthUrl,
    clientIds,
    maxUploadMB: 50
  }
}

function normalizeConfigTool (raw) {
  let clean = String(raw || '').trim().toLowerCase()
  let extFormat = ''
  if (clean.endsWith('.yaml')) {
    extFormat = 'yaml'
    clean = clean.slice(0, -5)
  } else if (clean.endsWith('.yml')) {
    extFormat = 'yaml'
    clean = clean.slice(0, -4)
  } else if (clean.endsWith('.json')) {
    extFormat = 'json'
    clean = clean.slice(0, -5)
  }
  if (clean === 'uri_handler' || clean === 'urihandler') clean = 'uri-handler'
  if (clean === 'bundle' || clean === 'workstation') clean = 'all'
  if (clean === 'godot' || studio.configTools.includes(clean)) {
    return { tool: clean, extFormat }
  }
  const err = new Error('Unsupported configuration tool profile')
  err.statusCode = 400
  throw err
}

function parseConfigFormat (explicitFormat, extFormat, acceptHeader) {
  const raw = String(explicitFormat || extFormat || '').trim().toLowerCase()
  if (raw) {
    if (raw === 'json') return 'json'
    if (raw === 'yaml' || raw === 'yml') return 'yaml'
    const err = new Error('Unsupported configuration format (expected json or yaml)')
    err.statusCode = 400
    throw err
  }
  const acc = String(acceptHeader || '').toLowerCase()
  if (acc.includes('application/yaml') || acc.includes('application/x-yaml') || acc.includes('text/yaml') || acc.includes('text/x-yaml')) {
    return 'yaml'
  }
  return 'json'
}

function encodeConfig (payload, format, headerComments = []) {
  const norm = parseConfigFormat(format, '', '')
  if (norm === 'json') {
    return {
      contentType: 'application/json; charset=utf-8',
      body: `${JSON.stringify(payload, null, 2)}\n`
    }
  }
  const comments = headerComments
    .map(line => String(line || '').replace(/[\r\n]+/g, ' ').trim())
    .filter(Boolean)
    .map(line => `# ${line}\n`)
    .join('')
  const dumped = yaml.safeDump(JSON.parse(JSON.stringify(payload)), {
    noRefs: true,
    lineWidth: 120
  })
  return {
    contentType: 'application/yaml; charset=utf-8',
    body: `${comments}${dumped}`
  }
}

function extractUserGroups (user) {
  const authGroups = (typeof WIKI !== 'undefined' && WIKI.auth && WIKI.auth.groups) || {}
  const ids = new Set()
  const names = []
  for (const item of (user && Array.isArray(user.groups) ? user.groups : [])) {
    if (Number.isInteger(item)) {
      ids.add(item)
      const gName = authGroups[item] && authGroups[item].name
      if (gName && !names.includes(gName)) names.push(gName)
    } else if (item && typeof item === 'object') {
      if (Number.isInteger(item.id)) {
        ids.add(item.id)
        const gName = authGroups[item.id] && authGroups[item.id].name
        if (gName && !names.includes(gName)) names.push(gName)
      }
      if (typeof item.name === 'string' && item.name && !names.includes(item.name)) {
        names.push(item.name)
      }
    }
  }
  return { ids, names }
}

function subjectIdentity (user) {
  const { names } = extractUserGroups(user)
  const isAdmin = Boolean(
    (typeof WIKI !== 'undefined' && WIKI.auth && typeof WIKI.auth.checkAccess === 'function' && WIKI.auth.checkAccess(user, ['manage:system'])) ||
    names.includes(studio.authentikGroups.cLevel) ||
    names.includes(studio.authentikGroups.admins)
  )
  return {
    sub: user.providerId || (user.id ? `wiki:${user.id}` : 'api'),
    wikiUserId: Number.isInteger(user.id) ? user.id : undefined,
    username: user.username || (user.email ? String(user.email).split('@')[0] : undefined),
    name: user.name || undefined,
    email: user.email || undefined,
    groups: names,
    isAdmin
  }
}

function effectiveLoreGrantFor (state, user) {
  studio.upgradeState(state)
  const { ids, names } = extractUserGroups(user)
  const ident = subjectIdentity(user)
  const lore = state.lore || {}
  const actionSet = new Set()
  const addActions = list => {
    for (const item of (list || [])) {
      if (studio.loreActions.includes(item)) actionSet.add(item)
    }
  }
  let canCreateRepo = false
  if (ident.isAdmin || state.ownerId === user.id) {
    addActions(studio.loreActions)
    canCreateRepo = true
  }
  for (const entry of (state.access || [])) {
    if (entry.userId && entry.userId === user.id) {
      addActions(studio.normalizeLoreActions(undefined, entry.role))
    }
  }
  for (const entry of (state.groupAccess || [])) {
    if (ids.has(entry.groupId) || (entry.groupName && names.includes(entry.groupName))) {
      addActions(Array.isArray(entry.loreActions) ? entry.loreActions : studio.normalizeLoreActions(undefined, entry.role))
      if (entry.loreCanCreateRepo) canCreateRepo = true
    }
  }
  const isPartner = names.includes(studio.authentikGroups.partners)
  const isEmployee = names.includes(studio.authentikGroups.employees)
  if (lore.visibility !== 'restricted' && !isPartner && isEmployee) {
    addActions(['read', 'write'])
  }
  const actions = studio.loreActions.filter(a => actionSet.has(a))
  return {
    projectSlug: lore.projectSlug,
    visibility: lore.visibility || 'internal',
    remoteBase: lore.remoteBase || studio.loreDefaults.remoteBase,
    bridgeUrl: lore.bridgeUrl || studio.loreDefaults.bridgeUrl,
    authUrl: lore.authUrl || studio.loreDefaults.authUrl,
    actions,
    canRead: actions.some(a => ['read', 'write', 'admin', 'owner'].includes(a)),
    canWrite: actions.some(a => ['write', 'admin', 'owner'].includes(a)),
    canCreateRepo
  }
}

function buildStudioBlock (opts) {
  return {
    name: 'Rogue Fairy Studios',
    apiUrl: opts.studioApiUrl,
    intranetUrl: opts.intranetUrl,
    graphqlUrl: `${opts.intranetUrl}/graphql`
  }
}

function buildOidcApp (opts, slug, redirectPort = 0) {
  const issuer = `${opts.oidcBaseUrl}/application/o/${slug}`
  const clientId = opts.clientIds[slug] || (slug === 'rfs-intranet' ? 'intranet-rfs' : slug)
  const app = {
    baseUrl: opts.oidcBaseUrl,
    appSlug: slug,
    clientId,
    issuerUrl: issuer,
    oidcConfigurationUrl: `${issuer}/.well-known/openid-configuration`,
    authorizationEndpoint: `${opts.oidcBaseUrl}/application/o/authorize/`,
    tokenEndpoint: `${opts.oidcBaseUrl}/application/o/token/`,
    userinfoEndpoint: `${opts.oidcBaseUrl}/application/o/userinfo/`,
    deviceAuthorizationEndpoint: `${opts.oidcBaseUrl}/application/o/device/`,
    revocationEndpoint: `${opts.oidcBaseUrl}/application/o/revoke/`,
    scopes: 'openid profile email offline_access',
    pkceMethod: 'S256',
    loopbackHost: '127.0.0.1',
    redirectPort,
    redirectPath: '/callback',
    loopbackRedirectRegex: '^http://127\\.0\\.0\\.1(:[0-9]+)?/callback$'
  }
  if (redirectPort > 0) {
    app.redirectUri = `http://127.0.0.1:${redirectPort}/callback`
  }
  return app
}

function buildLoreBlock (opts, stateLore = null) {
  const remoteBase = (stateLore && stateLore.remoteBase) ? stateLore.remoteBase : opts.loreRemoteBase
  const bridgeUrl = (stateLore && stateLore.bridgeUrl) ? stateLore.bridgeUrl : opts.loreBridgeUrl
  const authUrl = (stateLore && stateLore.authUrl) ? stateLore.authUrl : opts.loreAuthUrl
  const projectSlug = (stateLore && stateLore.projectSlug) || undefined
  return {
    driverId: 'lore',
    remoteBase,
    bridgeUrl,
    authUrl,
    cliAppSlug: 'rfs-lore-cli',
    ciAppSlug: 'rfs-lore-ci',
    casCompression: 'zstd',
    namespaceFormat: '<project>/<repo>',
    projectSlug,
    namespace: projectSlug ? `${projectSlug}/*` : undefined,
    visibility: (stateLore && stateLore.visibility) || undefined,
    loginCommand: ['lore', 'auth', 'login', '--auth-url', bridgeUrl]
  }
}

function loreAccessLevel (grant) {
  if ((grant.actions || []).some(a => a === 'admin' || a === 'owner')) return 'admin'
  if (grant.canWrite) return 'read-write'
  if (grant.canRead) return 'read-only'
  return 'none'
}

function summarizeRepositories (state, role, loreGrant, user, vcsFilter = '') {
  const canEditStudio = role === 'manager' || role === 'editor'
  const out = []
  for (const r of (state.repositories || [])) {
    if (vcsFilter && r.system !== vcsFilter) continue
    const canRead = r.system === 'lore' ? loreGrant.canRead : true
    const canWrite = r.system === 'lore' ? loreGrant.canWrite : canEditStudio
    if (!canRead) continue
    let accessLevel = 'read-only'
    if (r.system === 'lore') {
      accessLevel = loreAccessLevel(loreGrant)
    } else if (role === 'manager') {
      accessLevel = 'admin'
    } else if (canWrite) {
      accessLevel = 'read-write'
    }
    const cloneCommand = r.remoteUri ?
      (r.system === 'lore' ? ['lore', 'clone', r.remoteUri] : (r.system === 'git' ? ['git', 'clone', r.remoteUri] : [])) :
      []
    const status = r.status || {}
    const locks = (status.locks || []).map(lk => ({
      path: lk.path || '',
      owner: lk.owner || '',
      branch: lk.branch || undefined,
      lockedAt: lk.lockedAt || undefined,
      ownedByUser: Boolean(lk.owner && [user.username, user.email, user.name].some(c => c && String(c).toLowerCase() === String(lk.owner).toLowerCase()))
    }))
    out.push({
      id: r.id,
      name: r.name,
      slug: r.slug,
      system: r.system,
      remoteUri: r.remoteUri || '',
      loreProject: r.loreProject || undefined,
      loreRepo: r.loreRepo || undefined,
      loreFullName: r.loreFullName || undefined,
      defaultBranch: r.defaultBranch || 'main',
      workspacePath: r.workspacePath || '.',
      accessLevel,
      canRead,
      canWrite,
      cloneCommand,
      branch: status.branch || undefined,
      latestRevision: status.revision || undefined,
      clean: status.clean !== false,
      lockCount: locks.length,
      locks
    })
  }
  return out
}

function buildBootstrapCommands (loreBlk, repos) {
  const cmds = []
  if (repos.some(r => r.system === 'lore') && Array.isArray(loreBlk.loginCommand)) {
    cmds.push([...loreBlk.loginCommand])
  }
  for (const r of repos) {
    if (!r.cloneCommand || !r.cloneCommand.length) continue
    const cmd = [...r.cloneCommand]
    if (r.workspacePath && r.workspacePath !== '.') cmd.push(r.workspacePath)
    cmds.push(cmd)
  }
  return cmds
}

function buildStudioConfigCatalog (user, opts = resolveEnvOptions()) {
  const pathPrefix = opts.studioApiUrl.endsWith('/p/studio') ? '/configs/' : '/v1/configs/'
  return {
    schemaVersion: CONFIG_SCHEMA_VERSION,
    scope: 'studio',
    studio: buildStudioBlock(opts),
    subject: subjectIdentity(user),
    profiles: TOOL_CATALOG.map(m => ({
      tool: m.tool,
      title: m.title,
      description: m.description,
      recommended: ['aetherion', 'geppetto', 'lore'].includes(m.tool),
      jsonUrl: `${opts.studioApiUrl}${pathPrefix}${m.tool}?format=json`,
      yamlUrl: `${opts.studioApiUrl}${pathPrefix}${m.tool}?format=yaml`
    }))
  }
}

function buildGeneralistAetherion (toolKey, genAt, studioBlk, ident, opts) {
  const oidc = buildOidcApp(opts, 'aetherion', 0)
  const lore = buildLoreBlock(opts)
  return {
    schemaVersion: CONFIG_SCHEMA_VERSION,
    generatedAt: genAt,
    scope: 'studio',
    tool: toolKey,
    studio: studioBlk,
    subject: ident,
    authentik: oidc,
    lore,
    engine: {
      key: toolKey,
      name: toolKey === 'godot' ? 'Godot Engine (Aetherion Compatible)' : 'Aetherion Engine',
      repository: 'git@github.com:RogueFairyStudios/aetherion.git',
      httpsRepository: 'https://github.com/RogueFairyStudios/aetherion.git',
      branch: 'aetherion',
      upstreamGodotBase: '4.6.0-dev'
    },
    editorSettings: {
      'network/authentik/issuer_url': oidc.issuerUrl,
      'network/authentik/client_id': oidc.clientId,
      'network/authentik/scopes': oidc.scopes,
      'network/authentik/studio_api_url': studioBlk.apiUrl,
      'network/authentik/redirect_port': 0,
      'network/authentik/persist_session': true,
      'network/authentik/auto_refresh': true
    },
    environment: {
      AETHERION_AUTHENTIK_ISSUER_URL: oidc.issuerUrl,
      AETHERION_AUTHENTIK_CLIENT_ID: oidc.clientId,
      AETHERION_AUTHENTIK_SCOPES: oidc.scopes,
      AETHERION_STUDIO_API_URL: studioBlk.apiUrl
    }
  }
}

function buildGeneralistGeppetto (genAt, studioBlk, ident, opts) {
  const oidc = buildOidcApp(opts, 'geppetto', 0)
  const lore = buildLoreBlock(opts)
  return {
    schemaVersion: CONFIG_SCHEMA_VERSION,
    generatedAt: genAt,
    scope: 'studio',
    tool: 'geppetto',
    studio: studioBlk,
    subject: ident,
    authentik: oidc,
    lore,
    kritaVersioningSettings: {
      configGroup: 'Versioning',
      driverId: 'lore',
      serverAddress: lore.remoteBase,
      authServerUrl: lore.bridgeUrl,
      authentikIssuerUrl: oidc.issuerUrl,
      authentikClientId: oidc.clientId,
      projectsRoot: '~/Documents/Geppetto',
      neverSyncedPatterns: ['*.kra~', '.*-autosave.kra', '*-autosave.kra']
    },
    rogueStudioPlugin: {
      clientApp: 'geppetto',
      apiUrl: studioBlk.apiUrl,
      graphqlUrl: studioBlk.graphqlUrl,
      workspacesEndpoint: `${studioBlk.apiUrl}/v1/me/workspaces?client=geppetto&vcs=lore`
    },
    environment: {
      STUDIO_API_URL: studioBlk.apiUrl,
      STUDIO_URL: studioBlk.graphqlUrl,
      STUDIO_CLIENT_APP: 'geppetto',
      LORE_REMOTE_BASE: lore.remoteBase,
      LORE_AUTH_URL: lore.bridgeUrl
    }
  }
}

function buildStudioToolConfig (rawTool, user, opts = resolveEnvOptions()) {
  const { tool } = normalizeConfigTool(rawTool)
  const studioBlk = buildStudioBlock(opts)
  const ident = subjectIdentity(user)
  const genAt = new Date().toISOString()
  const filenameBase = `roguefairy-${tool}`
  const headers = [
    `Rogue Fairy Studios — Generalist Tool Configuration (${tool})`,
    `Schema: ${CONFIG_SCHEMA_VERSION} | Generated: ${genAt}`
  ]

  let doc
  if (tool === 'authentik') {
    const applications = {}
    for (const slug of ['aetherion', 'geppetto', 'rfs-intranet', 'rfs-lore-cli', 'rfs-lore-ci', 'goethe', 'animafaber', 'fontpit', 'carnifex', 'rfs-studio']) {
      applications[slug] = buildOidcApp(opts, slug, slug === 'animafaber' ? 44200 : 0)
    }
    doc = {
      schemaVersion: CONFIG_SCHEMA_VERSION,
      generatedAt: genAt,
      scope: 'studio',
      tool: 'authentik',
      studio: studioBlk,
      subject: ident,
      authentik: {
        baseUrl: opts.oidcBaseUrl,
        issuerTemplate: `${opts.oidcBaseUrl}/application/o/{slug}/`,
        authorizationEndpoint: `${opts.oidcBaseUrl}/application/o/authorize/`,
        tokenEndpoint: `${opts.oidcBaseUrl}/application/o/token/`,
        userinfoEndpoint: `${opts.oidcBaseUrl}/application/o/userinfo/`,
        deviceAuthorizationEndpoint: `${opts.oidcBaseUrl}/application/o/device/`,
        revocationEndpoint: `${opts.oidcBaseUrl}/application/o/revoke/`,
        pkceMethod: 'S256',
        defaultScopes: ['openid', 'profile', 'email', 'offline_access'],
        loopbackRedirectRegex: '^http://127\\.0\\.0\\.1(:[0-9]+)?/callback$',
        groups: { ...studio.authentikGroups },
        applications
      }
    }
  } else if (tool === 'aetherion' || tool === 'godot') {
    doc = buildGeneralistAetherion(tool, genAt, studioBlk, ident, opts)
  } else if (tool === 'geppetto') {
    doc = buildGeneralistGeppetto(genAt, studioBlk, ident, opts)
  } else if (tool === 'lore') {
    const lore = buildLoreBlock(opts)
    doc = {
      schemaVersion: CONFIG_SCHEMA_VERSION,
      generatedAt: genAt,
      scope: 'studio',
      tool: 'lore',
      studio: studioBlk,
      subject: ident,
      authentik: buildOidcApp(opts, 'rfs-lore-cli', 0),
      ciAuthentik: buildOidcApp(opts, 'rfs-lore-ci', 0),
      lore,
      environment: {
        LORE_REMOTE_BASE: lore.remoteBase,
        LORE_AUTH_URL: lore.bridgeUrl,
        STUDIO_API_URL: studioBlk.apiUrl
      }
    }
  } else if (tool === 'goethe') {
    doc = {
      schemaVersion: CONFIG_SCHEMA_VERSION,
      generatedAt: genAt,
      scope: 'studio',
      tool: 'goethe',
      studio: studioBlk,
      subject: ident,
      authentik: buildOidcApp(opts, 'goethe', 0),
      lore: buildLoreBlock(opts),
      goethe: {
        schemaVersion: 1,
        archiveExtension: '.gdkg',
        compression: 'zstd',
        uriScheme: 'goethe',
        defaultDirs: { dialoguesDir: 'dialogues', portraitsDir: 'portraits', localesDir: 'locales' }
      },
      environment: { STUDIO_API_URL: studioBlk.apiUrl, STUDIO_URL: studioBlk.graphqlUrl, STUDIO_CLIENT_APP: 'goethe' }
    }
  } else if (tool === 'carnifex') {
    doc = {
      schemaVersion: CONFIG_SCHEMA_VERSION,
      generatedAt: genAt,
      scope: 'studio',
      tool: 'carnifex',
      studio: studioBlk,
      subject: ident,
      authentik: buildOidcApp(opts, 'carnifex', 0),
      lore: buildLoreBlock(opts),
      carnifex: { uriScheme: 'carnifex', maxUploadMB: opts.maxUploadMB, requireApprovedCredits: true },
      environment: { STUDIO_API_URL: studioBlk.apiUrl, STUDIO_URL: studioBlk.graphqlUrl, STUDIO_CLIENT_APP: 'carnifex' }
    }
  } else if (tool === 'animafaber') {
    doc = {
      schemaVersion: CONFIG_SCHEMA_VERSION,
      generatedAt: genAt,
      scope: 'studio',
      tool: 'animafaber',
      studio: studioBlk,
      subject: ident,
      authentik: buildOidcApp(opts, 'animafaber', 44200),
      lore: buildLoreBlock(opts),
      animafaber: { uriScheme: 'animafaber', loopbackCallback: 'http://127.0.0.1:44200/callback' },
      environment: { STUDIO_API_URL: studioBlk.apiUrl, STUDIO_URL: studioBlk.graphqlUrl, STUDIO_CLIENT_APP: 'animafaber' }
    }
  } else if (tool === 'fontpit') {
    doc = {
      schemaVersion: CONFIG_SCHEMA_VERSION,
      generatedAt: genAt,
      scope: 'studio',
      tool: 'fontpit',
      studio: studioBlk,
      subject: ident,
      authentik: buildOidcApp(opts, 'fontpit', 0),
      lore: buildLoreBlock(opts),
      fontpit: { uriScheme: 'fontpit', supportedFormats: ['ttf', 'otf', 'woff2', 'fnt'] },
      environment: { STUDIO_API_URL: studioBlk.apiUrl, STUDIO_URL: studioBlk.graphqlUrl, STUDIO_CLIENT_APP: 'fontpit' }
    }
  } else if (tool === 'uri-handler') {
    doc = {
      schemaVersion: CONFIG_SCHEMA_VERSION,
      generatedAt: genAt,
      scope: 'studio',
      tool: 'uri-handler',
      studio: studioBlk,
      subject: ident,
      uriHandler: {
        schemes: ['aetherion', 'godot', 'geppetto', 'goethe', 'lore', 'lores', 'carnifex', 'animafaber', 'fontpit'],
        apiUrl: studioBlk.apiUrl,
        intranetUrl: studioBlk.intranetUrl,
        authentikBaseUrl: opts.oidcBaseUrl
      }
    }
  } else if (tool === 'all') {
    doc = {
      schemaVersion: CONFIG_SCHEMA_VERSION,
      generatedAt: genAt,
      scope: 'studio',
      tool: 'all',
      studio: studioBlk,
      subject: ident,
      lore: buildLoreBlock(opts),
      tools: {
        aetherion: buildGeneralistAetherion('aetherion', genAt, studioBlk, ident, opts),
        geppetto: buildGeneralistGeppetto(genAt, studioBlk, ident, opts),
        lore: buildStudioToolConfig('lore', user, opts).doc,
        goethe: buildStudioToolConfig('goethe', user, opts).doc,
        carnifex: buildStudioToolConfig('carnifex', user, opts).doc,
        animafaber: buildStudioToolConfig('animafaber', user, opts).doc,
        fontpit: buildStudioToolConfig('fontpit', user, opts).doc,
        uriHandler: buildStudioToolConfig('uri-handler', user, opts).doc
      }
    }
  }
  return { doc, filenameBase, headers }
}

async function resolveLocalProject (projectRef, user) {
  const ref = String(projectRef || '').trim()
  if (!ref) {
    const err = new Error('Project unavailable')
    err.statusCode = 404
    throw err
  }
  let row = await WIKI.models.studioProjects.query().findById(ref)
  if (row) {
    const state = studio.upgradeState(JSON.parse(row.state))
    const role = roleFor(state, user)
    if (!role) {
      const err = new Error('Project unavailable')
      err.statusCode = 404
      throw err
    }
    return { project: row, state, role }
  }
  const allRows = await WIKI.models.studioProjects.query()
  for (const candidate of allRows) {
    const state = studio.upgradeState(JSON.parse(candidate.state))
    const role = roleFor(state, user)
    if (!role) continue
    const slug = (state.lore && state.lore.projectSlug) || studio.deriveLoreProjectSlug(state.name)
    if (slug.toLowerCase() === ref.toLowerCase() || studio.deriveLoreProjectSlug(state.name).toLowerCase() === ref.toLowerCase()) {
      return { project: candidate, state, role }
    }
  }
  const err = new Error('Project unavailable')
  err.statusCode = 404
  throw err
}

function buildProjectMeta (projectRow, state, loreGrant, opts) {
  const slug = loreGrant.projectSlug || studio.deriveLoreProjectSlug(state.name)
  const engine = studio.engines.includes(state.engine) ? state.engine : studio.defaultEngineForKind(state.kind)
  const meta = {
    id: projectRow.id,
    revision: projectRow.revision,
    slug,
    name: state.name,
    kind: state.kind,
    engine,
    usesAetherion: engine === 'aetherion',
    phase: state.phase,
    archived: Boolean(state.archived),
    intranetUrl: `${opts.intranetUrl}/p/studio?project=${projectRow.id}`,
    geppettoUri: `geppetto://project/${slug}`,
    goetheUri: `goethe://project/${slug}`
  }
  if (state.kind === 'digital' || state.kind === 'hybrid') {
    meta.godotUri = `godot://project/${slug}`
    if (engine === 'aetherion') meta.aetherionUri = `aetherion://project/${slug}`
  }
  return meta
}

function buildProjectConfigCatalog (projectRow, state, user, opts = resolveEnvOptions()) {
  studio.upgradeState(state)
  const loreGrant = effectiveLoreGrantFor(state, user)
  const projMeta = buildProjectMeta(projectRow, state, loreGrant, opts)
  const projPrefix = opts.studioApiUrl.endsWith('/p/studio') ?
    `/projects/${projMeta.slug}/configs/` :
    `/v1/projects/${projMeta.slug}/configs/`
  return {
    schemaVersion: CONFIG_SCHEMA_VERSION,
    scope: 'project',
    studio: buildStudioBlock(opts),
    subject: subjectIdentity(user),
    project: projMeta,
    profiles: TOOL_CATALOG.map(m => ({
      tool: m.tool,
      title: m.title,
      description: m.description,
      recommended: m.tool === 'geppetto' || m.tool === 'lore' || (m.tool === 'aetherion' && projMeta.engine === 'aetherion'),
      jsonUrl: `${opts.studioApiUrl}${projPrefix}${m.tool}?format=json`,
      yamlUrl: `${opts.studioApiUrl}${projPrefix}${m.tool}?format=yaml`
    }))
  }
}

function deriveGeppettoFolders (repos, artifacts) {
  const folders = []
  const add = raw => {
    const clean = String(raw || '').replace(/\\/g, '/').replace(/^\/+|\/+$/g, '').trim()
    if (!clean || clean === '.') return
    const first = clean.split('/')[0]
    if (first && !folders.some(f => f.toLowerCase() === first.toLowerCase())) folders.push(first)
  }
  for (const r of repos) {
    if (r.system === 'lore' && r.loreRepo) add(r.loreRepo)
    else if (r.workspacePath && r.workspacePath !== '.') add(r.workspacePath)
  }
  for (const a of artifacts) {
    if (a.assetPath) add(a.assetPath)
  }
  return folders.length ? folders : ['art', 'characters', 'animations', 'environments', 'ui', 'capsules']
}

function buildProjectToolConfig (projectRow, state, role, rawTool, user, opts = resolveEnvOptions()) {
  const { tool } = normalizeConfigTool(rawTool)
  studio.upgradeState(state)
  const studioBlk = buildStudioBlock(opts)
  const ident = subjectIdentity(user)
  const loreGrant = effectiveLoreGrantFor(state, user)
  const loreBlk = buildLoreBlock(opts, state.lore)
  const projMeta = buildProjectMeta(projectRow, state, loreGrant, opts)

  const { ids: groupIds, names: groupNames } = extractUserGroups(user)
  const groupGrants = []
  for (const entry of (state.groupAccess || [])) {
    const gName = entry.groupName || entry.name || ''
    if (role !== 'manager' && !groupIds.has(entry.groupId) && !(gName && groupNames.includes(gName))) {
      continue
    }
    groupGrants.push({
      groupId: entry.groupId || undefined,
      groupName: gName,
      studioRole: entry.role,
      loreActions: entry.loreActions || [],
      loreCanCreateRepo: Boolean(entry.loreCanCreateRepo)
    })
  }

  const matchedParticipants = (state.participants || []).filter(p => !p.endedAt && p.userId && p.userId === user.id)
  const participantIds = matchedParticipants.map(p => p.id)
  const participantRoles = [...new Set(matchedParticipants.reduce((acc, p) => acc.concat(p.roles || []), []))]

  const accessBlk = {
    studioRole: role,
    canManage: role === 'manager',
    canEdit: role === 'manager' || role === 'editor',
    canPublishArtwork: role === 'manager' || role === 'editor',
    participantIds,
    participantRoles,
    lore: loreGrant,
    groupGrants
  }

  const allRepos = summarizeRepositories(state, role, loreGrant, user, '')
  const loreRepos = summarizeRepositories(state, role, loreGrant, user, 'lore')
  const genAt = new Date().toISOString()
  const filenameBase = `${projMeta.slug}-${tool}`
  const headers = [
    `Rogue Fairy Studios — Project Configuration: ${projMeta.name} (${tool})`,
    `Project Slug: ${projMeta.slug} | Engine: ${projMeta.engine} | Role: ${role} | Lore Access: ${loreAccessLevel(loreGrant)}`,
    `Schema: ${CONFIG_SCHEMA_VERSION} | Generated: ${genAt}`
  ]

  let doc
  if (tool === 'aetherion' || tool === 'godot') {
    const oidc = buildOidcApp(opts, 'aetherion', 0)
    const gitRepo = allRepos.find(r => r.system === 'git' && r.workspacePath)
    const primaryWs = (gitRepo && gitRepo.workspacePath) || (allRepos[0] && allRepos[0].workspacePath) || '.'
    const openTasks = (state.tasks || []).filter(t => t.status !== 'done')
    doc = {
      schemaVersion: CONFIG_SCHEMA_VERSION,
      generatedAt: genAt,
      scope: 'project',
      tool,
      studio: studioBlk,
      subject: ident,
      project: projMeta,
      access: accessBlk,
      authentik: oidc,
      lore: loreBlk,
      engine: {
        projectEngine: projMeta.engine,
        usesAetherion: projMeta.usesAetherion,
        engineRepository: 'git@github.com:RogueFairyStudios/aetherion.git',
        httpsRepository: 'https://github.com/RogueFairyStudios/aetherion.git',
        branch: 'aetherion',
        upstreamGodotBase: '4.6.0-dev',
        godotUri: projMeta.godotUri,
        aetherionUri: projMeta.aetherionUri
      },
      editorSettings: {
        'network/authentik/issuer_url': oidc.issuerUrl,
        'network/authentik/client_id': oidc.clientId,
        'network/authentik/scopes': oidc.scopes,
        'network/authentik/studio_api_url': studioBlk.apiUrl,
        'network/authentik/redirect_port': 0,
        'network/authentik/persist_session': true,
        'network/authentik/auto_refresh': true
      },
      projectSettings: {
        'application/config/name': projMeta.name,
        'network/authentik/issuer_url': oidc.issuerUrl,
        'network/authentik/client_id': oidc.clientId,
        'network/authentik/scopes': oidc.scopes
      },
      workspace: {
        recommendedRoot: `~/Projects/${projMeta.slug}`,
        primaryWorkspacePath: primaryWs,
        bootstrapCommands: buildBootstrapCommands(loreBlk, allRepos),
        openTaskCount: openTasks.length,
        assignedOpenTaskCount: openTasks.filter(t => participantIds.includes(t.assigneeId)).length
      },
      repositories: allRepos,
      environment: {
        AETHERION_AUTHENTIK_ISSUER_URL: oidc.issuerUrl,
        AETHERION_AUTHENTIK_CLIENT_ID: oidc.clientId,
        AETHERION_AUTHENTIK_SCOPES: oidc.scopes,
        AETHERION_STUDIO_API_URL: studioBlk.apiUrl,
        STUDIO_API_URL: studioBlk.apiUrl,
        STUDIO_PROJECT_ID: projMeta.id
      }
    }
  } else if (tool === 'geppetto') {
    const oidc = buildOidcApp(opts, 'geppetto', 0)
    const repos = [...loreRepos, ...allRepos.filter(r => !loreRepos.some(lr => lr.id === r.id))]
    const artArtifacts = (state.artifacts || [])
      .filter(a => a.track === 'art_2d' || a.tool === 'geppetto' || String(a.assetPath || '').toLowerCase().endsWith('.kra'))
      .map(a => ({
        id: a.id,
        name: a.name,
        kind: a.kind,
        track: a.track,
        stage: a.stage,
        tool: a.tool,
        toolUri: a.toolUri || undefined,
        assetPath: a.assetPath || undefined
      }))
    const storefrontSlots = []
    for (const target of (state.platformTargets || [])) {
      if (target.archived) continue
      for (const slot of (target.assets || [])) {
        if (slot.status === 'done' || slot.status === 'na') continue
        if (String(slot.format || '').includes('MP4')) continue
        storefrontSlots.push({
          targetId: target.id,
          targetName: target.name,
          storefront: target.storefront,
          slotId: slot.id,
          name: slot.name,
          dimensions: slot.dimensions,
          format: slot.format,
          status: slot.status,
          required: slot.required !== false,
          toolUri: slot.toolUri || undefined
        })
      }
    }
    doc = {
      schemaVersion: CONFIG_SCHEMA_VERSION,
      generatedAt: genAt,
      scope: 'project',
      tool: 'geppetto',
      studio: studioBlk,
      subject: ident,
      project: projMeta,
      access: accessBlk,
      authentik: oidc,
      lore: loreBlk,
      kritaVersioningSettings: {
        configGroup: 'Versioning',
        driverId: 'lore',
        serverAddress: loreBlk.remoteBase,
        authServerUrl: loreBlk.bridgeUrl,
        authentikIssuerUrl: oidc.issuerUrl,
        authentikClientId: oidc.clientId,
        projectsRoot: '~/Documents/Geppetto',
        neverSyncedPatterns: ['*.kra~', '.*-autosave.kra', '*-autosave.kra'],
        projectEntry: {
          projectId: projMeta.id,
          projectSlug: projMeta.slug,
          projectName: projMeta.name,
          driverId: 'lore',
          serverAddress: loreBlk.remoteBase,
          authServerUrl: loreBlk.bridgeUrl,
          localWorkspaceRoot: `~/Documents/Geppetto/${projMeta.slug}`,
          accessLevel: loreAccessLevel(loreGrant),
          writable: loreGrant.canWrite,
          canCreateRepo: loreGrant.canCreateRepo,
          allowedActions: loreGrant.actions,
          folders: deriveGeppettoFolders(repos, artArtifacts)
        }
      },
      rogueStudioPlugin: {
        clientApp: 'geppetto',
        apiUrl: studioBlk.apiUrl,
        graphqlUrl: studioBlk.graphqlUrl,
        projectId: projMeta.id,
        projectSlug: projMeta.slug,
        workspacesEndpoint: `${studioBlk.apiUrl}/v1/me/workspaces?client=geppetto&vcs=lore`
      },
      repositories: repos,
      artPipeline: {
        geppettoUri: projMeta.geppettoUri,
        bootstrapCommands: buildBootstrapCommands(loreBlk, repos),
        artifacts: artArtifacts,
        storefrontSlots
      },
      environment: {
        STUDIO_API_URL: studioBlk.apiUrl,
        STUDIO_URL: studioBlk.graphqlUrl,
        STUDIO_PROJECT_ID: projMeta.id,
        STUDIO_CLIENT_APP: 'geppetto',
        LORE_REMOTE_BASE: loreBlk.remoteBase,
        LORE_AUTH_URL: loreBlk.bridgeUrl
      }
    }
  } else if (tool === 'lore') {
    doc = {
      schemaVersion: CONFIG_SCHEMA_VERSION,
      generatedAt: genAt,
      scope: 'project',
      tool: 'lore',
      studio: studioBlk,
      subject: ident,
      project: projMeta,
      access: accessBlk,
      authentik: buildOidcApp(opts, 'rfs-lore-cli', 0),
      lore: {
        remoteBase: loreBlk.remoteBase,
        bridgeUrl: loreBlk.bridgeUrl,
        authUrl: loreBlk.authUrl,
        projectSlug: projMeta.slug,
        namespace: `${projMeta.slug}/*`,
        visibility: loreGrant.visibility,
        accessLevel: loreAccessLevel(loreGrant),
        allowedActions: loreGrant.actions,
        canRead: loreGrant.canRead,
        canWrite: loreGrant.canWrite,
        canCreateRepo: loreGrant.canCreateRepo,
        loginCommand: loreBlk.loginCommand,
        casCompression: 'zstd',
        bootstrapScript: buildBootstrapCommands(loreBlk, loreRepos)
      },
      repositories: loreRepos.map(r => ({
        ...r,
        configToml: `remote_url = ${JSON.stringify(r.remoteUri)}\n`
      })),
      environment: {
        LORE_REMOTE_BASE: loreBlk.remoteBase,
        LORE_AUTH_URL: loreBlk.bridgeUrl,
        STUDIO_API_URL: studioBlk.apiUrl,
        STUDIO_PROJECT_ID: projMeta.id
      }
    }
  } else if (tool === 'goethe') {
    doc = {
      schemaVersion: CONFIG_SCHEMA_VERSION,
      generatedAt: genAt,
      scope: 'project',
      tool: 'goethe',
      studio: studioBlk,
      subject: ident,
      project: projMeta,
      access: accessBlk,
      authentik: buildOidcApp(opts, 'goethe', 0),
      lore: loreBlk,
      gproj: {
        filename: `${projMeta.slug}.gproj.yaml`,
        name: projMeta.name,
        version: '0.1.0',
        author: 'Rogue Fairy Studios',
        default_locale: 'en',
        dialogues_dir: 'dialogues',
        portraits_dir: 'portraits',
        locales_dir: 'locales'
      },
      gdkgBuild: {
        outputArchive: `build/${projMeta.slug}.gdkg`,
        compression: 'zstd',
        schemaVersion: 1
      },
      repositories: allRepos,
      environment: {
        STUDIO_API_URL: studioBlk.apiUrl,
        STUDIO_URL: studioBlk.graphqlUrl,
        STUDIO_PROJECT_ID: projMeta.id,
        STUDIO_CLIENT_APP: 'goethe'
      }
    }
  } else if (tool === 'authentik') {
    doc = {
      schemaVersion: CONFIG_SCHEMA_VERSION,
      generatedAt: genAt,
      scope: 'project',
      tool: 'authentik',
      studio: studioBlk,
      subject: ident,
      project: projMeta,
      access: accessBlk,
      authentik: {
        baseUrl: opts.oidcBaseUrl,
        loopbackRedirectRegex: '^http://127\\.0\\.0\\.1(:[0-9]+)?/callback$',
        loreProjectAttribute: projMeta.slug,
        loreVisibility: loreGrant.visibility,
        effectiveLoreActions: loreGrant.actions,
        applications: {
          aetherion: buildOidcApp(opts, 'aetherion', 0),
          geppetto: buildOidcApp(opts, 'geppetto', 0),
          loreCli: buildOidcApp(opts, 'rfs-lore-cli', 0),
          goethe: buildOidcApp(opts, 'goethe', 0)
        }
      },
      authentikBlueprintYaml: role === 'manager' ? studio.exportAuthentikBlueprint(state) : undefined
    }
  } else if (tool === 'carnifex') {
    const approvedList = (state.credits || []).filter(c => c.status === 'approved')
    const lastCredits = approvedList.length ? approvedList[approvedList.length - 1] : null
    doc = {
      schemaVersion: CONFIG_SCHEMA_VERSION,
      generatedAt: genAt,
      scope: 'project',
      tool: 'carnifex',
      studio: studioBlk,
      subject: ident,
      project: projMeta,
      access: accessBlk,
      authentik: buildOidcApp(opts, 'carnifex', 0),
      lore: loreBlk,
      carnifex: {
        projectEngine: projMeta.engine,
        maxUploadMB: opts.maxUploadMB,
        latestApprovedCredits: lastCredits ? { snapshotId: lastCredits.id, releaseId: lastCredits.releaseId, checksum: lastCredits.checksum } : null,
        releases: (state.releases || []).map(r => ({ id: r.id, name: r.name, publishedAt: r.publishedAt || null }))
      },
      repositories: allRepos,
      environment: {
        STUDIO_API_URL: studioBlk.apiUrl,
        STUDIO_URL: studioBlk.graphqlUrl,
        STUDIO_PROJECT_ID: projMeta.id,
        STUDIO_CLIENT_APP: 'carnifex'
      }
    }
  } else if (tool === 'animafaber' || tool === 'fontpit') {
    doc = {
      schemaVersion: CONFIG_SCHEMA_VERSION,
      generatedAt: genAt,
      scope: 'project',
      tool,
      studio: studioBlk,
      subject: ident,
      project: projMeta,
      access: accessBlk,
      authentik: buildOidcApp(opts, tool, tool === 'animafaber' ? 44200 : 0),
      lore: loreBlk,
      repositories: allRepos,
      environment: {
        STUDIO_API_URL: studioBlk.apiUrl,
        STUDIO_URL: studioBlk.graphqlUrl,
        STUDIO_PROJECT_ID: projMeta.id,
        STUDIO_CLIENT_APP: tool
      }
    }
  } else if (tool === 'uri-handler') {
    const repositoryMapping = {}
    for (const r of allRepos) {
      repositoryMapping[r.slug] = {
        name: r.name,
        system: r.system,
        remoteUri: r.remoteUri,
        loreFullName: r.loreFullName,
        workspacePath: r.workspacePath,
        accessLevel: r.accessLevel
      }
    }
    doc = {
      schemaVersion: CONFIG_SCHEMA_VERSION,
      generatedAt: genAt,
      scope: 'project',
      tool: 'uri-handler',
      studio: studioBlk,
      subject: ident,
      project: projMeta,
      access: accessBlk,
      uriHandler: {
        schemes: ['aetherion', 'godot', 'geppetto', 'goethe', 'lore', 'lores', 'carnifex', 'animafaber', 'fontpit'],
        recommendedRoot: `~/Projects/${projMeta.slug}`,
        repositoryMapping
      }
    }
  } else if (tool === 'all') {
    doc = {
      schemaVersion: CONFIG_SCHEMA_VERSION,
      generatedAt: genAt,
      scope: 'project',
      tool: 'all',
      studio: studioBlk,
      subject: ident,
      project: projMeta,
      access: accessBlk,
      lore: loreBlk,
      repositories: allRepos,
      tools: {
        aetherion: buildProjectToolConfig(projectRow, state, role, 'aetherion', user, opts).doc,
        geppetto: buildProjectToolConfig(projectRow, state, role, 'geppetto', user, opts).doc,
        lore: buildProjectToolConfig(projectRow, state, role, 'lore', user, opts).doc,
        goethe: buildProjectToolConfig(projectRow, state, role, 'goethe', user, opts).doc,
        carnifex: buildProjectToolConfig(projectRow, state, role, 'carnifex', user, opts).doc
      }
    }
  }

  return { doc, filenameBase, headers }
}

module.exports = {
  CONFIG_SCHEMA_VERSION,
  TOOL_CATALOG,
  normalizeConfigTool,
  parseConfigFormat,
  encodeConfig,
  resolveLocalProject,
  buildStudioConfigCatalog,
  buildStudioToolConfig,
  buildProjectConfigCatalog,
  buildProjectToolConfig
}
