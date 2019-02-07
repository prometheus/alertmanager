module Url.Parser.Internal exposing
  ( QueryParser(..)
  )


import Dict


type QueryParser a =
  Parser (Dict.Dict String (List String) -> a)
