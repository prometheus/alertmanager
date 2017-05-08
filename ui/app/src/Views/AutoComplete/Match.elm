module Views.AutoComplete.Match exposing (levenshtein, levenshteinFromStrings)


levenshteinFromStrings : Bool -> String -> String -> Int
levenshteinFromStrings consecutive source target =
    levenshtein consecutive (String.toList source) (String.toList target)


levenshtein : Bool -> List comparable -> List comparable -> Int
levenshtein consecutive source target =
    case ( source, target ) of
        ( source, [] ) ->
            List.length source

        ( [], target ) ->
            List.length target

        ( src_hd :: src_tail, tgt_hd :: tgt_tail ) ->
            if src_hd == tgt_hd then
                -- If we have a direct match, we want to bump this up the list.
                -- If it's also part of a consecutive match, let's add even
                -- more.
                (levenshtein True src_tail tgt_tail)
                    - (scoreMatch
                        * if consecutive then
                            consecutiveBonus
                          else
                            1
                      )
            else
                Maybe.withDefault 0
                    (List.minimum
                        [ (levenshtein False src_tail target) + 1
                        , (levenshtein False source tgt_tail) + 1
                        , (levenshtein False src_tail tgt_tail) + 1
                        ]
                    )



-- The first character in the typed pattern usually has more significance
-- than the rest so it's important that it appears at special positions where
-- bonus points are given. e.g. "to-go" vs. "ongoing" on "og" or on "ogo".
-- The amount of the extra bonus should be limited so that the gap penalty is
-- still respected.


bonusFirstCharMultiplier =
    2


consecutiveBonus =
    2


scoreMatch =
    160
