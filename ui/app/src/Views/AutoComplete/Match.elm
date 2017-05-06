module Views.AutoComplete.Match exposing (levenshtein, levenshteinFromStrings)


levenshteinFromStrings : String -> String -> Int
levenshteinFromStrings source target =
    levenshtein (String.toList source) (String.toList target)


levenshtein : List comparable -> List comparable -> Int
levenshtein source target =
    case ( source, target ) of
        ( source, [] ) ->
            List.length source

        ( [], target ) ->
            List.length target

        ( src_hd :: src_tail, tgt_hd :: tgt_tail ) ->
            if src_hd == tgt_hd then
                levenshtein src_tail tgt_tail
            else
                Maybe.withDefault 0
                    (List.minimum
                        [ (levenshtein src_tail target) + 1
                        , (levenshtein source tgt_tail) + 1
                        , (levenshtein src_tail tgt_tail) + 1
                        ]
                    )
