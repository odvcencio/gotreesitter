proc classify {status} {
    switch -exact -- $status {
        pass      { return PASS }
        fallback  -
        skip      { return DECLINE }
        default   { return UNKNOWN }
    }
}
puts [classify pass]
