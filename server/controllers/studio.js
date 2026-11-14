const express = require('express')
const multer = require('multer')
const crypto = require('crypto')
const path = require('path')
const { actor, getProject } = require('../helpers/studio-access')
const studioRemote = require('../helpers/studio-remote')
const studioConfigs = require('../helpers/studio-configs')
const router = express.Router()

/* global WIKI */

function isTruthyQuery (raw) {
  return ['1', 'true', 'yes', 'on'].includes(String(raw || '').trim().toLowerCase())
}

function buildConfigQueryString (query) {
  const params = new URLSearchParams()
  for (const key of ['format', 'download', 'user']) {
    if (query && typeof query[key] === 'string' && query[key].trim()) {
      params.set(key, query[key].trim())
    }
  }
  const qs = params.toString()
  return qs ? `?${qs}` : ''
}

function sendConfigResponse (res, { contentType, body, filenameBase, format, download }) {
  res.set('Cache-Control', 'private, no-store')
  res.set('X-Content-Type-Options', 'nosniff')
  if (download && filenameBase) {
    const ext = format === 'yaml' ? 'yaml' : 'json'
    res.attachment(`${filenameBase}.${ext}`)
  }
  return res.type(contentType).send(body)
}

function relayRemoteConfig (res, remote) {
  res.set('Cache-Control', 'private, no-store')
  res.set('X-Content-Type-Options', 'nosniff')
  if (remote.contentDisposition) {
    res.set('Content-Disposition', remote.contentDisposition)
  }
  return res.status(remote.statusCode).type(remote.contentType).send(remote.body)
}

router.get('/p/studio/configs', async (req, res, next) => {
  let user
  try {
    user = await actor({ req })
  } catch (error) {
    return res.sendStatus(403)
  }
  try {
    if (studioRemote.isEnabled()) {
      const qs = buildConfigQueryString(req.query)
      const remote = await studioRemote.fetchConfig(`/v1/configs${qs}`, { user, accept: req.headers.accept })
      return relayRemoteConfig(res, remote)
    }
    const format = studioConfigs.parseConfigFormat(req.query.format, '', req.headers.accept)
    const catalog = studioConfigs.buildStudioConfigCatalog(user)
    const encoded = studioConfigs.encodeConfig(catalog, format, ['Rogue Fairy Studios — Studio Configuration Catalog'])
    return sendConfigResponse(res, {
      ...encoded,
      filenameBase: 'roguefairy-configs-catalog',
      format,
      download: isTruthyQuery(req.query.download)
    })
  } catch (error) {
    if (error && error.statusCode) return res.status(error.statusCode).json({ error: error.message })
    next(error)
  }
})

router.get('/p/studio/configs/:tool', async (req, res, next) => {
  let user
  try {
    user = await actor({ req })
  } catch (error) {
    return res.sendStatus(403)
  }
  try {
    if (studioRemote.isEnabled()) {
      const qs = buildConfigQueryString(req.query)
      const remote = await studioRemote.fetchConfig(`/v1/configs/${encodeURIComponent(req.params.tool)}${qs}`, {
        user,
        accept: req.headers.accept
      })
      return relayRemoteConfig(res, remote)
    }
    const { tool, extFormat } = studioConfigs.normalizeConfigTool(req.params.tool)
    const format = studioConfigs.parseConfigFormat(req.query.format, extFormat, req.headers.accept)
    const { doc, filenameBase, headers } = studioConfigs.buildStudioToolConfig(tool, user)
    const encoded = studioConfigs.encodeConfig(doc, format, headers)
    return sendConfigResponse(res, {
      ...encoded,
      filenameBase,
      format,
      download: isTruthyQuery(req.query.download)
    })
  } catch (error) {
    if (error && error.statusCode) return res.status(error.statusCode).json({ error: error.message })
    next(error)
  }
})

router.get('/p/studio/projects/:projectRef/configs', async (req, res, next) => {
  let user
  try {
    user = await actor({ req })
  } catch (error) {
    return res.sendStatus(403)
  }
  try {
    if (studioRemote.isEnabled()) {
      const qs = buildConfigQueryString(req.query)
      const remote = await studioRemote.fetchConfig(
        `/v1/projects/${encodeURIComponent(req.params.projectRef)}/configs${qs}`,
        { user, accept: req.headers.accept }
      )
      return relayRemoteConfig(res, remote)
    }
    const { project, state } = await studioConfigs.resolveLocalProject(req.params.projectRef, user)
    const format = studioConfigs.parseConfigFormat(req.query.format, '', req.headers.accept)
    const catalog = studioConfigs.buildProjectConfigCatalog(project, state, user)
    const encoded = studioConfigs.encodeConfig(catalog, format, [
      `Rogue Fairy Studios — Project Configuration Catalog: ${catalog.project.name}`
    ])
    return sendConfigResponse(res, {
      ...encoded,
      filenameBase: `${catalog.project.slug}-configs-catalog`,
      format,
      download: isTruthyQuery(req.query.download)
    })
  } catch (error) {
    if (error && error.statusCode) return res.status(error.statusCode).json({ error: error.message })
    next(error)
  }
})

router.get('/p/studio/projects/:projectRef/configs/:tool', async (req, res, next) => {
  let user
  try {
    user = await actor({ req })
  } catch (error) {
    return res.sendStatus(403)
  }
  try {
    if (studioRemote.isEnabled()) {
      const qs = buildConfigQueryString(req.query)
      const remote = await studioRemote.fetchConfig(
        `/v1/projects/${encodeURIComponent(req.params.projectRef)}/configs/${encodeURIComponent(req.params.tool)}${qs}`,
        { user, accept: req.headers.accept }
      )
      return relayRemoteConfig(res, remote)
    }
    const { tool, extFormat } = studioConfigs.normalizeConfigTool(req.params.tool)
    const format = studioConfigs.parseConfigFormat(req.query.format, extFormat, req.headers.accept)
    const { project, state, role } = await studioConfigs.resolveLocalProject(req.params.projectRef, user)
    const { doc, filenameBase, headers } = studioConfigs.buildProjectToolConfig(project, state, role, tool, user)
    const encoded = studioConfigs.encodeConfig(doc, format, headers)
    return sendConfigResponse(res, {
      ...encoded,
      filenameBase,
      format,
      download: isTruthyQuery(req.query.download)
    })
  } catch (error) {
    if (error && error.statusCode) return res.status(error.statusCode).json({ error: error.message })
    next(error)
  }
})

router.post('/p/studio/files/:projectId', async (req, res, next) => {
  try {
    const user = await actor({ req })
    req.studioActor = user
    if (studioRemote.isEnabled()) {
      const remote = await studioRemote.getProject(req.params.projectId, user)
      const state = JSON.parse(remote.state)
      if (!remote.canEdit || state.archived) return res.sendStatus(403)
    } else {
      const { role, state } = await getProject(req.params.projectId, user)
      if (!['manager', 'editor'].includes(role) || state.archived) return res.sendStatus(403)
    }
    const limit = Math.min((WIKI.config && WIKI.config.uploads && WIKI.config.uploads.maxFileSize) || 15728640, 52428800)
    multer({ storage: multer.memoryStorage(), limits: { fileSize: limit, files: 1, fields: 0 } }).single('file')(req, res, error => {
      if (error) return res.status(400).json({ error: 'File exceeds configured limits or upload is invalid' })
      next()
    })
  } catch (error) { res.sendStatus(403) }
}, async (req, res, next) => {
  try {
    if (!req.file) return res.status(400).json({ error: 'File required' })
    const projectId = req.params.projectId
    if (studioRemote.isEnabled()) {
      const uploaded = await studioRemote.uploadFile(projectId, req.file.originalname, req.file.buffer, req.studioActor)
      return res.json({ id: uploaded.id, checksum: uploaded.checksum, path: `/p/studio/files/${projectId}/${uploaded.id}` })
    }
    const checksum = crypto.createHash('sha256').update(req.file.buffer).digest('hex')
    let file = await WIKI.models.knex('studioFiles').select('id').where({ projectId, checksum }).first()
    if (!file) {
      const id = crypto.randomUUID()
      try {
        await WIKI.models.knex('studioFiles').insert({ id, projectId, checksum, name: path.basename(req.file.originalname).slice(0, 255), createdAt: new Date().toISOString(), data: req.file.buffer })
        file = { id }
      } catch (error) {
        file = await WIKI.models.knex('studioFiles').select('id').where({ projectId, checksum }).first()
        if (!file) throw error
      }
    }
    res.json({ id: file.id, checksum, path: `/p/studio/files/${projectId}/${file.id}` })
  } catch (error) { next(error) }
})

router.get('/p/studio/files/:projectId/:fileId', async (req, res, next) => {
  let user
  try {
    user = await actor({ req })
    if (!studioRemote.isEnabled()) {
      await getProject(req.params.projectId, user)
    }
  } catch (error) { return res.sendStatus(403) }
  try {
    if (studioRemote.isEnabled()) {
      const dl = await studioRemote.downloadFile(req.params.projectId, req.params.fileId, user)
      if (dl.statusCode === 404) return res.sendStatus(404)
      if (dl.statusCode >= 400) return res.sendStatus(403)
      res.set('Cache-Control', 'private, no-store')
      res.set('X-Content-Type-Options', 'nosniff')
      res.set('Content-Security-Policy', "default-src 'none'; sandbox")
      res.attachment(dl.filename)
      return res.type('application/octet-stream').send(dl.data)
    }
    const file = await WIKI.models.knex('studioFiles').where({ id: req.params.fileId, projectId: req.params.projectId }).first()
    if (!file) return res.sendStatus(404)
    res.set('Cache-Control', 'private, no-store')
    res.set('X-Content-Type-Options', 'nosniff')
    res.set('Content-Security-Policy', "default-src 'none'; sandbox")
    res.attachment(file.name)
    res.type('application/octet-stream').send(file.data)
  } catch (error) { next(error) }
})

module.exports = router
