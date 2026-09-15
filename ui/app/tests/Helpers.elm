module Helpers exposing (isNotEmptyTrimmedAlphabetWord)

import String


isNotEmptyTrimmedAlphabetWord : String -> Bool
isNotEmptyTrimmedAlphabetWord string =
    let
        stringLength =
            String.length string
    in
    stringLength
        /= 0
        && String.length (String.filter isLetter string)
        == stringLength


isLetter : Char -> Bool
isLetter char =
    String.contains (String.fromChar char) lowerCaseAlphabet
        || String.contains (String.fromChar char) upperCaseAlphabet


lowerCaseAlphabet : String
lowerCaseAlphabet =
    "abcdefghijklmnopqrstuvwxyz"


upperCaseAlphabet : String
upperCaseAlphabet =
    "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
