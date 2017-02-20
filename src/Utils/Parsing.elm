module Utils.Parsing exposing (..)

import Utils.Types
import Regex


parseLabels : Maybe String -> Maybe Utils.Types.Matchers
parseLabels labelsString =
    case labelsString of
        Just ls ->
            let
                replace =
                    (Regex.replace Regex.All (Regex.regex "{|}|\"|\\s") (\_ -> ""))

                matches =
                    String.split "," (replace ls)

                labels =
                    List.filterMap
                        (\m ->
                            let
                                label =
                                    String.split "=" m
                            in
                                if List.length label == 2 then
                                    let
                                        name =
                                            Maybe.withDefault "" (List.head label)

                                        value =
                                            Maybe.withDefault "" (List.head <| List.reverse label)
                                    in
                                        Just { name = name, value = value, isRegex = False }
                                else
                                    Nothing
                        )
                        matches
            in
                Just labels

        Nothing ->
            Nothing
