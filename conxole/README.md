conxole relay
=============

  - modified basic relay catered for Conxole's use case
  - mostly acting as a private relay for Conxole 

running
-------

grab a binary from the releases page and run it with the environment variable POSTGRESQL_DATABASE set to some postgres url:

    POSTGRESQL_DATABASE=postgres://name:pass@localhost:5432/dbname ./conxole

it also accepts a HOST and a PORT environment variables.
