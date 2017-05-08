module Views.AutoComplete.Match exposing (jaro, jaroWinkler, commonPrefix)

import Utils.List exposing (zip)
import Char


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


{-|

    Adapted from https://blog.art-of-coding.eu/comparing-strings-with-metrics-in-haskell/
-}
jaro : String -> String -> Float
jaro s1 s2 =
    if s1 == s2 then
        1.0
    else
        let
            l1 =
                String.length s1

            l2 =
                String.length s2

            z2 =
                zip (List.range 1 l2) (String.toList s2)
                    |> List.map (Tuple.mapSecond Char.toCode)

            searchLength =
                ((max l1 l2) % 2)

            m =
                zip (List.range 1 l1) (String.toList s1)
                    |> List.map (Tuple.mapSecond Char.toCode)
                    |> List.map (charMatch searchLength z2)
                    |> List.foldl (++) []

            ml =
                List.length m

            t =
                m
                    |> List.map (transposition z2 >> toFloat >> (flip (/) 2.0))
                    |> List.sum

            ml1 =
                toFloat ml / toFloat l1

            ml2 =
                toFloat ml / toFloat l2

            mtm =
                if ml == 0 then
                    0
                else
                    (toFloat ml - t) / toFloat ml
        in
            (1 / 3) * (ml1 + ml2 + mtm)


winkler : String -> String -> Float -> Float
winkler s1 s2 jaro =
    if s1 == "" || s2 == "" then
        0.0
    else if s1 == s2 then
        1.0
    else
        let
            l =
                commonPrefix s1 s2
                    |> String.length
                    |> toFloat

            p =
                0.1
        in
            jaro + ((l * p) * (1.0 - jaro))


jaroWinkler : String -> String -> Float
jaroWinkler s1 s2 =
    if s1 == "" || s2 == "" then
        0.0
    else if s1 == s2 then
        1.0
    else
        jaro s1 s2
            |> winkler s1 s2


commonPrefix : String -> String -> String
commonPrefix s1 s2 =
    if s1 == "" || s2 == "" then
        ""
    else if s1 == s2 then
        String.left 4 s1
    else
        cp (String.toList s1) (String.toList s2) []
            |> String.fromList


cp : List Char -> List Char -> List Char -> List Char
cp l1 l2 acc =
    if List.length acc == 4 then
        acc
    else
        case l1 of
            [] ->
                acc

            x :: xs ->
                case l2 of
                    [] ->
                        acc

                    y :: ys ->
                        if x == y then
                            x :: cp xs ys acc
                        else
                            acc


charMatch : Int -> List ( Int, Int ) -> ( Int, Int ) -> List ( Int, Int )
charMatch far list ( p, q ) =
    -- TODO(w0rm): Is there a way to define this so it's not so strictly bound
    -- to type Int?
    list
        |> List.filter
            (\( x, y ) ->
                x >= p - far && x <= p + far && y == q
            )


transposition : List ( Int, Int ) -> ( Int, Int ) -> Int
transposition list ( p, q ) =
    list
        |> List.filter
            (\( x, y ) ->
                p /= x && q == y
            )
        |> List.length



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
    16
