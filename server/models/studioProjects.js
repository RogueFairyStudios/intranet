const Model = require('objection').Model

module.exports = class StudioProject extends Model {
  static get tableName () { return 'studioProjects' }
}
