module Utils.Filter exposing
    ( Filter
    , MatchOperator(..)
    , Matcher
    , SilenceFormGetParams
    , convertFilterMatcher
    , emptySilenceFormGetParams
    , generateAPIQueryString
    , generateQueryParam
    , generateQueryString
    , nullFilter
    , parseFilter
    , parseGroup
    , parseMatcher
    , silencePreviewFilter
    , stringifyFilter
    , stringifyGroup
    , stringifyMatcher
    )

import Char
import Data.Matcher
import Parser exposing ((|.), (|=), Parser, Trailing(..))
import Set
import Url exposing (percentEncode)


type alias Filter =
    { text : Maybe String
    , group : Maybe String
    , customGrouping : Bool
    , receiver : Maybe String
    , showSilenced : Maybe Bool
    , showInhibited : Maybe Bool
    , showActive : Maybe Bool
    }


nullFilter : Filter
nullFilter =
    { text = Nothing
    , group = Nothing
    , customGrouping = False
    , receiver = Nothing
    , showSilenced = Nothing
    , showInhibited = Nothing
    , showActive = Nothing
    }


generateQueryParam : String -> Maybe String -> Maybe String
generateQueryParam name =
    Maybe.map (percentEncode >> (++) (name ++ "="))


generateQueryString : Filter -> String
generateQueryString { receiver, customGrouping, showSilenced, showInhibited, showActive, text, group } =
    let
        parts =
            [ ( "silenced", Maybe.withDefault False showSilenced |> boolToString |> Just )
            , ( "inhibited", Maybe.withDefault False showInhibited |> boolToString |> Just )
            , ( "active", Maybe.withDefault True showActive |> boolToString |> Just )
            , ( "filter", emptyToNothing text )
            , ( "receiver", emptyToNothing receiver )
            , ( "group", group )
            , ( "customGrouping", boolToMaybeString customGrouping )
            ]
                |> List.filterMap (\( a, b ) -> generateQueryParam a b)
    in
    if List.length parts > 0 then
        parts
            |> String.join "&"
            |> (++) "?"

    else
        ""


generateAPIQueryString : Filter -> String
generateAPIQueryString { receiver, showSilenced, showInhibited, showActive, text, group } =
    let
        filter_ =
            case parseFilter (Maybe.withDefault "" text) of
                Just matchers_ ->
                    List.map (stringifyMatcher >> Just >> Tuple.pair "filter") matchers_

                Nothing ->
                    []

        parts =
            filter_
                ++ [ ( "silenced", Maybe.withDefault False showSilenced |> boolToString |> Just )
                   , ( "inhibited", Maybe.withDefault False showInhibited |> boolToString |> Just )
                   , ( "active", Maybe.withDefault True showActive |> boolToString |> Just )
                   , ( "receiver", emptyToNothing receiver )
                   , ( "group", group )
                   ]
                |> List.filterMap (\( a, b ) -> generateQueryParam a b)
    in
    if List.length parts > 0 then
        parts
            |> String.join "&"
            |> (++) "?"

    else
        ""


boolToMaybeString : Bool -> Maybe String
boolToMaybeString b =
    if b then
        Just "true"

    else
        Nothing


boolToString : Bool -> String
boolToString b =
    if b then
        "true"

    else
        "false"


emptyToNothing : Maybe String -> Maybe String
emptyToNothing str =
    case str of
        Just "" ->
            Nothing

        _ ->
            str


type alias Matcher =
    { key : String
    , op : MatchOperator
    , value : String
    }


convertAPIMatcher : Data.Matcher.Matcher -> Matcher
convertAPIMatcher { name, value, isRegex } =
    { key = name
    , value = value
    , op =
        if isRegex then
            RegexMatch

        else
            Eq
    }


type MatchOperator
    = Eq
    | NotEq
    | RegexMatch
    | NotRegexMatch


matchers : List ( String, MatchOperator )
matchers =
    [ ( "=~", RegexMatch )
    , ( "!~", NotRegexMatch )
    , ( "=", Eq )
    , ( "!=", NotEq )
    ]


parseFilter : String -> Maybe (List Matcher)
parseFilter =
    Parser.run filter
        >> Result.toMaybe


parseMatcher : String -> Maybe Matcher
parseMatcher =
    Parser.run matcher
        >> Result.toMaybe


stringifyGroup : List String -> Maybe String
stringifyGroup list =
    if List.isEmpty list then
        Just ""

    else if list == [ "alertname" ] then
        Nothing

    else
        Just (String.join "," list)


parseGroup : Maybe String -> List String
parseGroup maybeGroup =
    case maybeGroup of
        Nothing ->
            [ "alertname" ]

        Just something ->
            String.split "," something
                |> List.filter (String.length >> (<) 0)


stringifyFilter : List Matcher -> String
stringifyFilter matchers_ =
    case matchers_ of
        [] ->
            ""

        list ->
            (list
                |> List.map stringifyMatcher
                |> String.join ", "
                |> (++) "{"
            )
                ++ "}"


stringifyMatcher : Matcher -> String
stringifyMatcher { key, op, value } =
    key
        ++ (matchers
                |> List.filter (Tuple.second >> (==) op)
                |> List.head
                |> Maybe.map Tuple.first
                |> Maybe.withDefault ""
           )
        ++ "\""
        ++ value
        ++ "\""


convertFilterMatcher : Matcher -> Data.Matcher.Matcher
convertFilterMatcher { key, op, value } =
    { name = key
    , value = value
    , isRegex = op == RegexMatch
    }


filter : Parser (List Matcher)
filter =
    Parser.succeed identity
        |= Parser.sequence
            { start = "{"
            , separator = ","
            , end = "}"
            , spaces = Parser.spaces
            , item = item
            , trailing = Forbidden
            }
        |. Parser.end


matcher : Parser Matcher
matcher =
    Parser.succeed identity
        |. Parser.spaces
        |= item
        |. Parser.spaces
        |. Parser.end


item : Parser Matcher
item =
    Parser.succeed Matcher
        |= Parser.variable
            { start = isVarChar
            , inner = isVarChar
            , reserved = Set.empty
            }
        |= (matchers
                |> List.map
                    (\( keyword, matcher_ ) ->
                        Parser.succeed matcher_
                            |. Parser.keyword keyword
                    )
                |> Parser.oneOf
           )
        |= string '"'


string : Char -> Parser String
string separator =
    Parser.succeed ()
        |. Parser.token (String.fromChar separator)
        |. Parser.loop separator stringHelp
        |> Parser.getChompedString
        -- Remove quotes
        |> Parser.map (String.dropLeft 1 >> String.dropRight 1)


stringHelp : Char -> Parser (Parser.Step Char ())
stringHelp separator =
    Parser.oneOf
        [ Parser.succeed (Parser.Done ())
            |. Parser.token (String.fromChar separator)
        , Parser.succeed (Parser.Loop separator)
            |. Parser.chompIf (\char -> char == '\\')
            |. Parser.chompIf (\_ -> True)
        , Parser.succeed (Parser.Loop separator)
            |. Parser.chompIf (\char -> char /= '\\' && char /= separator)
        ]


isVarChar : Char -> Bool
isVarChar char =
    Char.isLower char
        || Char.isUpper char
        || (char == '_')
        || Char.isDigit char


silencePreviewFilter : List Data.Matcher.Matcher -> Filter
silencePreviewFilter apiMatchers =
    { nullFilter
        | text =
            List.map convertAPIMatcher apiMatchers
                |> stringifyFilter
                |> Just
        , showSilenced = Just True
        , showInhibited = Just True
        , showActive = Just True
    }


type alias SilenceFormGetParams =
    { matchers : List Matcher
    , comment : String
    }


emptySilenceFormGetParams : SilenceFormGetParams
emptySilenceFormGetParams =
    { matchers = []
    , comment = ""
    }
