/** @jest-environment node */
const http = require('http')
const express = require('express')
const router = require('../controllers/studio')

describe('project file access', () => {
  let server
  let files
  let project
  beforeEach(async () => {
    files = []
    project = { ownerId: 8, access: [{ userId: 9, role: 'reader' }] }
    global.WIKI = {
      config: { uploads: { maxFileSize: 1024 } },
      auth: { checkAccess: () => false },
      models: {
        users: { query: () => ({ findById: async id => ({ id, isActive: true }) }) },
        studioProjects: { query: () => ({ findById: async () => ({ id: 'p', state: JSON.stringify(project) }) }) },
        knex: () => ({
          select () { return this },
          where (where) { this.condition = where; return this },
          async first () { return files.find(file => Object.keys(this.condition).every(key => file[key] === this.condition[key])) },
          async insert (file) { files.push(file) }
        })
      }
    }
    const app = express()
    app.use((req, res, next) => { req.user = { id: Number(req.headers['x-test-user']) }; next() })
    app.use(router)
    server = app.listen(0, '127.0.0.1')
    await new Promise(resolve => server.once('listening', resolve))
  })
  afterEach(async () => { await new Promise(resolve => server.close(resolve)); delete global.WIKI })
  function request (user, method, path, body) {
    return new Promise((resolve, reject) => {
      const req = http.request({ host: '127.0.0.1', port: server.address().port, method, path, headers: { 'x-test-user': user, 'Content-Type': 'multipart/form-data; boundary=studio-test' } }, res => {
        const chunks = []
        res.on('data', chunk => chunks.push(chunk))
        res.on('end', () => resolve({ status: res.statusCode, headers: res.headers, body: Buffer.concat(chunks).toString() }))
      })
      req.on('error', reject)
      req.end(body)
    })
  }
  const upload = content => `--studio-test\r\nContent-Disposition: form-data; name="file"; filename="art.txt"\r\nContent-Type: text/plain\r\n\r\n${content}\r\n--studio-test--\r\n`
  test('uploads are private, deduplicated and downloadable by project readers', async () => {
    const first = await request(8, 'POST', '/p/studio/files/p', upload('art'))
    const second = await request(8, 'POST', '/p/studio/files/p', upload('art'))
    expect(first.status).toBe(200)
    expect(JSON.parse(first.body).id).toBe(JSON.parse(second.body).id)
    expect(files).toHaveLength(1)
    const download = await request(9, 'GET', JSON.parse(first.body).path)
    expect(download.body).toBe('art')
    expect(download.headers['cache-control']).toBe('private, no-store')
  })
  test('outsiders cannot download and readers cannot upload', async () => {
    const uploadResult = await request(8, 'POST', '/p/studio/files/p', upload('art'))
    expect((await request(12, 'GET', JSON.parse(uploadResult.body).path)).status).toBe(403)
    expect((await request(9, 'POST', '/p/studio/files/p', upload('art'))).status).toBe(403)
  })
  test('configured upload size and archived projects are enforced', async () => {
    expect((await request(8, 'POST', '/p/studio/files/p', upload('a'.repeat(2048)))).status).toBe(400)
    project.archived = true
    expect((await request(8, 'POST', '/p/studio/files/p', upload('art'))).status).toBe(403)
    expect(files).toHaveLength(0)
  })
})
