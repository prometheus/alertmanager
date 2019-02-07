module MicroRandomExtra exposing (..)

{-| Most of these are copied from elm-random-extra.
-}

import Array exposing (Array)
import Random exposing (..)
import String


lengthString : Generator Char -> Int -> Generator String
lengthString charGenerator stringLength =
    list stringLength charGenerator
        |> map String.fromList


bool : Generator Bool
bool =
    int 0 1 |> map ((==) 0)


sample : List a -> Generator (Maybe a)
sample =
    let
        find k ys =
            case ys of
                [] ->
                    Nothing

                z :: zs ->
                    if k == 0 then
                        Just z

                    else
                        find (k - 1) zs
    in
    \xs -> map (\i -> find i xs) (int 0 (List.length xs - 1))


oneIn : Int -> Generator Bool
oneIn n =
    map ((==) 1) (int 1 n)


frequency : ( Float, Generator a ) -> List ( Float, Generator a ) -> Generator a
frequency firstPair restPairs =
    let
        total =
            List.sum <| List.map (abs << Tuple.first) (firstPair :: restPairs)

        pick ( k, g ) restChoices n =
            if n <= k then
                g

            else
                case restChoices of
                    [] ->
                        g

                    next :: rest ->
                        pick next rest (n - k)
    in
    float 0 total |> andThen (pick firstPair restPairs)
