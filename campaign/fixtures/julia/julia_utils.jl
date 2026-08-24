# Reduced from JuliaLang/julia at a176dec25429b497e304120acaff491639edab1b.
# Source path: base/ryu/utils.jl.
# Full source SHA-256: 51a8d2279c45469fe5d6e01d77125381c3884a57d0c8d5156e1f290014154e3f.
const BIG_MASK = (big(1) << 64) - 1

function generateinversetables()
    POW10_OFFSET_2 = Vector{UInt16}(undef, 68 + 1)
    MIN_BLOCK_2 = fill(0xff, 68 + 1)
    POW10_SPLIT_2 = Tuple{UInt64, UInt64, UInt64}[]
    lowerCutoff = big(1) << (54 + 8)
    for idx = 0:67
        POW10_OFFSET_2[idx + 1] = length(POW10_SPLIT_2)
        i = 0
        while true
            v = ((big(10)^(9 * (i + 1)) >> (-(120 - 16 * idx))) % (big(10)^9) << (120 + 16))
            if MIN_BLOCK_2[idx + 1] == 0xff && ((v * lowerCutoff) >> 128) == 0
                i += 1
                continue
            end
            if MIN_BLOCK_2[idx + 1] == 0xff
                MIN_BLOCK_2[idx + 1] = i
            end
            v == 0 && break
            push!(POW10_SPLIT_2, ((v & BIG_MASK) % UInt64, ((v >> 64) & BIG_MASK) % UInt64, ((v >> 128) & BIG_MASK) % UInt64))
            i += 1
        end
    end
end
