const express = require('express')
const multer = require('multer')
const crypto = require('crypto')
const path = require('path')
const { actor, getProject } = require('../helpers/studio-access')
const router = express.Router()

/* global WIKI */

router.post('/p/studio/files/:projectId', async (req, res, next) => {
  try {
    const { role, state } = await getProject(req.params.projectId, await actor({ req }))
    if (!['manager', 'editor'].includes(role) || state.archived) return res.sendStatus(403)
    const limit = Math.min(WIKI.config.uploads.maxFileSize || 15728640, 52428800)
    multer({ storage: multer.memoryStorage(), limits: { fileSize: limit, files: 1, fields: 0 } }).single('file')(req, res, error => {
      if (error) return res.status(400).json({ error: 'File exceeds configured limits or upload is invalid' })
      next()
    })
  } catch (error) { res.sendStatus(403) }
}, async (req, res, next) => {
  try {
    if (!req.file) return res.status(400).json({ error: 'File required' })
    const projectId = req.params.projectId
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
  try {
    await getProject(req.params.projectId, await actor({ req }))
  } catch (error) { return res.sendStatus(403) }
  try {
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
