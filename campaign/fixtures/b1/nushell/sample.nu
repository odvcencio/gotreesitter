def census [] {
    ls campaign/fixtures/b1
    | where type == dir
    | each { |row| $row.name }
    | length
}
census
