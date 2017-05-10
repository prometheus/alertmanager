module Utils.String exposing (capitalizeFirst)

import String
import Char


capitalizeFirst : String -> String
capitalizeFirst string =
    case String.uncons string of
        Nothing ->
            string

        Just ( char, rest ) ->
            String.cons (Char.toUpper char) rest
