module Runner.String.Format exposing (format)

import Diff exposing (Change(..))
import Test.Runner.Failure exposing (InvalidReason(..), Reason(..))


format : String -> Reason -> String
format description reason =
    case reason of
        Custom ->
            description

        Equality expected actual ->
            equalityToString { operation = description, expected = expected, actual = actual }

        Comparison first second ->
            verticalBar description first second

        TODO ->
            description

        Invalid BadDescription ->
            if description == "" then
                "The empty string is not a valid test description."

            else
                "This is an invalid test description: " ++ description

        Invalid _ ->
            description

        ListDiff expected actual ->
            listDiffToString 0
                description
                { expected = expected
                , actual = actual
                }
                { originalExpected = expected
                , originalActual = actual
                }

        CollectionDiff { expected, actual, extra, missing } ->
            let
                extraStr =
                    if List.isEmpty extra then
                        ""

                    else
                        "\nThese keys are extra: "
                            ++ (extra |> String.join ", " |> (\d -> "[ " ++ d ++ " ]"))

                missingStr =
                    if List.isEmpty missing then
                        ""

                    else
                        "\nThese keys are missing: "
                            ++ (missing |> String.join ", " |> (\d -> "[ " ++ d ++ " ]"))
            in
            String.join ""
                [ verticalBar description expected actual
                , "\n"
                , extraStr
                , missingStr
                ]


verticalBar : String -> String -> String -> String
verticalBar comparison expected actual =
    [ actual
    , "╵"
    , "│ " ++ comparison
    , "╷"
    , expected
    ]
        |> String.join "\n"


listDiffToString :
    Int
    -> String
    -> { expected : List String, actual : List String }
    -> { originalExpected : List String, originalActual : List String }
    -> String
listDiffToString index description { expected, actual } originals =
    case ( expected, actual ) of
        ( [], [] ) ->
            [ "Two lists were unequal previously, yet ended up equal later."
            , "This should never happen!"
            , "Please report this bug to https://github.com/elm-community/elm-test/issues - and include these lists: "
            , "\n"
            , Debug.toString originals.originalExpected
            , "\n"
            , Debug.toString originals.originalActual
            ]
                |> String.join ""

        ( first :: _, [] ) ->
            verticalBar (description ++ " was shorter than")
                (Debug.toString originals.originalExpected)
                (Debug.toString originals.originalActual)

        ( [], first :: _ ) ->
            verticalBar (description ++ " was longer than")
                (Debug.toString originals.originalExpected)
                (Debug.toString originals.originalActual)

        ( firstExpected :: restExpected, firstActual :: restActual ) ->
            if firstExpected == firstActual then
                -- They're still the same so far; keep going.
                listDiffToString (index + 1)
                    description
                    { expected = restExpected
                    , actual = restActual
                    }
                    originals

            else
                -- We found elements that differ; fail!
                String.join ""
                    [ verticalBar description
                        (Debug.toString originals.originalExpected)
                        (Debug.toString originals.originalActual)
                    , "\n\nThe first diff is at index "
                    , Debug.toString index
                    , ": it was `"
                    , firstActual
                    , "`, but `"
                    , firstExpected
                    , "` was expected."
                    ]


equalityToString : { operation : String, expected : String, actual : String } -> String
equalityToString { operation, expected, actual } =
    -- TODO make sure this looks reasonable for multiline strings
    let
        ( formattedExpected, belowFormattedExpected ) =
            Diff.diff (String.toList expected) (String.toList actual)
                |> List.map formatExpectedChange
                |> List.unzip

        ( formattedActual, belowFormattedActual ) =
            Diff.diff (String.toList actual) (String.toList expected)
                |> List.map formatActualChange
                |> List.unzip

        combinedExpected =
            String.join "\n"
                [ String.join "" formattedExpected
                , String.join "" belowFormattedExpected
                ]

        combinedActual =
            String.join "\n"
                [ String.join "" formattedActual
                , String.join "" belowFormattedActual
                ]
    in
    verticalBar operation combinedExpected combinedActual


formatExpectedChange : Change Char -> ( String, String )
formatExpectedChange diff =
    case diff of
        Added char ->
            ( "", "" )

        Removed char ->
            ( String.fromChar char, "▲" )

        NoChange char ->
            ( String.fromChar char, " " )


formatActualChange : Change Char -> ( String, String )
formatActualChange diff =
    case diff of
        Added char ->
            ( "", "" )

        Removed char ->
            ( "▼", String.fromChar char )

        NoChange char ->
            ( " ", String.fromChar char )
