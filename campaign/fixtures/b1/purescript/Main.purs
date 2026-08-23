module Census.Main where

import Prelude

classify :: String -> String
classify = case _ of
  "pass" -> "PASS"
  "skip" -> "SKIP"
  _ -> "FALLBACK"
