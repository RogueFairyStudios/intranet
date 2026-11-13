exports.up = async knex => {
  await knex.schema.createTable('studioProjects', table => {
    table.string('id', 36).primary()
    table.integer('revision').notNullable().defaultTo(1)
    table.text('state', 'longtext').notNullable()
  })
}

exports.down = async knex => {
  await knex.schema.dropTable('studioProjects')
}
