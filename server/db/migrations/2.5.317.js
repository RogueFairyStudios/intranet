exports.up = async knex => {
  await knex.schema.createTable('studioFiles', table => {
    table.string('id', 36).primary()
    table.string('projectId', 36).notNullable().references('id').inTable('studioProjects').onDelete('CASCADE')
    table.string('checksum', 64).notNullable()
    table.string('name', 255).notNullable()
    table.string('createdAt').notNullable()
    if (['mysql', 'mysql2'].includes(knex.client.config.client)) table.specificType('data', 'LONGBLOB').notNullable()
    else table.binary('data').notNullable()
    table.unique(['projectId', 'checksum'])
  })
}

exports.down = async knex => { await knex.schema.dropTable('studioFiles') }
