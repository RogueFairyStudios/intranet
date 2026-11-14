const http = require('http')
const https = require('https')
const crypto = require('crypto')
const path = require('path')

/* global WIKI */

function apiBaseUrl () {
  return String(process.env.STUDIO_API_URL || '').trim().replace(/\/+$/, '')
}

function internalKey () {
  return String(process.env.STUDIO_INTERNAL_KEY || '').trim()
}

function isEnabled () {
  return Boolean(apiBaseUrl() && internalKey())
}

function encodeActor (user) {
  if (!user) throw new Error('Unauthorized')
  const authGroups = (typeof WIKI !== 'undefined' && WIKI.auth && WIKI.auth.groups) || {}
  const groups = []
  for (const item of (Array.isArray(user.groups) ? user.groups : [])) {
    if (Number.isInteger(item)) {
      groups.push({ id: item, name: (authGroups[item] && authGroups[item].name) || '' })
    } else if (item && typeof item === 'object') {
      const gid = Number.isInteger(item.id) ? item.id : 0
      const gname = typeof item.name === 'string' && item.name ?
        item.name :
        ((gid && authGroups[gid] && authGroups[gid].name) || '')
      groups.push({ id: gid, name: gname })
    }
  }
  const permissions = Array.isArray(user.permissions) ? [...user.permissions] : []
  if (
    typeof WIKI !== 'undefined' &&
    WIKI.auth &&
    typeof WIKI.auth.checkAccess === 'function' &&
    WIKI.auth.checkAccess(user, ['manage:system']) &&
    !permissions.includes('manage:system')
  ) {
    permissions.push('manage:system')
  }
  const payload = {
    id: Number.isInteger(user.id) ? user.id : 0,
    apiId: Number.isInteger(user.apiId) ? user.apiId : 0,
    email: user.email || '',
    username: user.username || '',
    name: user.name || '',
    groups,
    permissions
  }
  return Buffer.from(JSON.stringify(payload), 'utf8').toString('base64url')
}

function rawRequest (method, urlPath, { user, headers = {}, body = null, timeoutMs = 15000 } = {}) {
  const base = apiBaseUrl()
  if (!base) return Promise.reject(new Error('Studio remote API is not configured'))
  const target = new URL(urlPath.replace(/^\/+/, ''), `${base}/`)
  const transport = target.protocol === 'https:' ? https : http

  const reqHeaders = {
    'X-Studio-Internal-Key': internalKey(),
    'X-Studio-Client': 'intranet',
    ...headers
  }
  if (user) {
    reqHeaders['X-Studio-Actor'] = encodeActor(user)
  }
  if (body && reqHeaders['Content-Length'] === undefined) {
    reqHeaders['Content-Length'] = Buffer.byteLength(body)
  }

  return new Promise((resolve, reject) => {
    const req = transport.request(target, { method, headers: reqHeaders, timeout: timeoutMs }, res => {
      const chunks = []
      res.on('data', chunk => chunks.push(chunk))
      res.on('end', () => {
        resolve({
          statusCode: res.statusCode || 500,
          headers: res.headers,
          buffer: Buffer.concat(chunks)
        })
      })
    })
    req.on('timeout', () => {
      req.destroy(new Error('Studio server request timed out'))
    })
    req.on('error', reject)
    if (body) req.write(body)
    req.end()
  })
}

async function jsonRequest (method, urlPath, { user, payload = undefined } = {}) {
  const headers = { Accept: 'application/json' }
  let body = null
  if (payload !== undefined) {
    body = Buffer.from(JSON.stringify(payload), 'utf8')
    headers['Content-Type'] = 'application/json'
  }
  const res = await rawRequest(method, urlPath, { user, headers, body })
  let parsed = null
  if (res.buffer.length > 0) {
    try {
      parsed = JSON.parse(res.buffer.toString('utf8'))
    } catch (err) {
      parsed = null
    }
  }
  if (res.statusCode >= 200 && res.statusCode < 300) {
    return parsed || {}
  }
  const message = (parsed && parsed.error) || (res.statusCode === 404 ? 'Project unavailable' : `Studio server error (${res.statusCode})`)
  const err = new Error(message)
  err.statusCode = res.statusCode
  throw err
}

async function listProjects (user) {
  const data = await jsonRequest('GET', '/v1/projects', { user })
  return Array.isArray(data.projects) ? data.projects : []
}

async function getProject (projectId, user) {
  return jsonRequest('GET', `/v1/projects/${encodeURIComponent(projectId)}`, { user })
}

async function createProject (name, kind, user, engine = '') {
  const payload = engine ? { name, kind, engine } : { name, kind }
  return jsonRequest('POST', '/v1/projects', { user, payload })
}

async function runCommand (projectId, revision, command, input, user) {
  return jsonRequest('POST', `/v1/projects/${encodeURIComponent(projectId)}/commands`, {
    user,
    payload: { revision, command, input }
  })
}

async function exportCredits (projectId, snapshotId, format, user) {
  const qs = format ? `?format=${encodeURIComponent(format)}` : ''
  const data = await jsonRequest('GET', `/v1/projects/${encodeURIComponent(projectId)}/credits/${encodeURIComponent(snapshotId)}${qs}`, { user })
  return data.content || ''
}

async function uploadFile (projectId, originalName, fileBuffer, user) {
  const boundary = `----RogueStudioBoundary${crypto.randomBytes(16).toString('hex')}`
  const safeName = path.basename(String(originalName || 'artifact.bin')).replace(/["\r\n]/g, '_').slice(0, 255) || 'artifact.bin'
  const preamble = Buffer.from(
    `--${boundary}\r\n` +
    `Content-Disposition: form-data; name="file"; filename="${safeName}"\r\n` +
    'Content-Type: application/octet-stream\r\n\r\n',
    'utf8'
  )
  const epilogue = Buffer.from(`\r\n--${boundary}--\r\n`, 'utf8')
  const body = Buffer.concat([preamble, fileBuffer, epilogue])

  const res = await rawRequest('POST', `/v1/projects/${encodeURIComponent(projectId)}/files`, {
    user,
    headers: {
      'Content-Type': `multipart/form-data; boundary=${boundary}`,
      Accept: 'application/json'
    },
    body,
    timeoutMs: 60000
  })
  let parsed = null
  try {
    parsed = JSON.parse(res.buffer.toString('utf8'))
  } catch (err) {
    parsed = null
  }
  if (res.statusCode >= 200 && res.statusCode < 300 && parsed) {
    return parsed
  }
  const err = new Error((parsed && parsed.error) || 'Upload failed')
  err.statusCode = res.statusCode
  throw err
}

async function downloadFile (projectId, fileId, user) {
  const res = await rawRequest('GET', `/v1/projects/${encodeURIComponent(projectId)}/files/${encodeURIComponent(fileId)}`, {
    user,
    timeoutMs: 60000
  })
  let filename = 'download.bin'
  const cd = String(res.headers['content-disposition'] || '')
  const match = /filename="?([^";]+)"?/i.exec(cd)
  if (match && match[1]) filename = path.basename(match[1])
  return {
    statusCode: res.statusCode,
    filename,
    data: res.buffer
  }
}

async function fetchConfig (urlPath, { user, accept = '' } = {}) {
  const headers = {}
  if (accept) headers.Accept = accept
  const res = await rawRequest('GET', urlPath, { user, headers, timeoutMs: 15000 })
  return {
    statusCode: res.statusCode,
    contentType: String(res.headers['content-type'] || 'application/json; charset=utf-8'),
    contentDisposition: String(res.headers['content-disposition'] || ''),
    body: res.buffer.toString('utf8')
  }
}

module.exports = {
  isEnabled,
  encodeActor,
  listProjects,
  getProject,
  createProject,
  runCommand,
  exportCredits,
  uploadFile,
  downloadFile,
  fetchConfig
}
