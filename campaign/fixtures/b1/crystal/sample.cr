class Census
  property counts = Hash(String, Int32).new(0)

  def bump(lang : String)
    @counts[lang] += 1
  end

  def total : Int32
    @counts.values.sum
  end
end

puts Census.new.tap { |c| c.bump("crystal") }.total
