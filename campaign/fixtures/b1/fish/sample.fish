function census
    set -l langs (ls grammars | string match -r '^[a-z]+')
    for lang in $langs
        echo "checking $lang"
    end
end
census | grep checking | wc -l
