module Utils.Filter
    exposing
        ( Matcher
        , MatchOperator(..)
        , Filter
        , nullFilter
        , generateQueryParam
        , generateQueryString
        , stringifyMatcher
        , stringifyFilter
        , parseFilter
        , parseMatcher
        )

import Http exposing (encodeUri)
import Parser exposing (Parser, (|.), (|=), zeroOrMore, ignore)
import Parser.LanguageKit as Parser exposing (Trailing(..))
import Char
import Set


type alias Filter =
    { text : Maybe String
    , receiver : Maybe Matcher
    , showSilenced : Maybe Bool
    }


nullFilter : Filter
nullFilter =
    { text = Nothing
    , receiver = Nothing
    , showSilenced = Nothing
    }


generateQueryParam : String -> Maybe String -> Maybe String
generateQueryParam name =
    Maybe.map (encodeUri >> (++) (name ++ "="))


generateQueryString : Filter -> String
generateQueryString { receiver, showSilenced, text } =
    -- TODO: Re-add receiver once it is parsed on the server side.
    [ ( "silenced", Maybe.map (toString >> String.toLower) showSilenced )
    , ( "filter", emptyToNothing text )
    ]
        |> List.filterMap (uncurry generateQueryParam)
        |> String.join "&"
        |> (++) "?"


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


stringifyFilter : List Matcher -> String
stringifyFilter matchers =
    case matchers of
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


filter : Parser (List Matcher)
filter =
    Parser.succeed identity
        |= Parser.record spaces item
        |. Parser.end


matcher : Parser Matcher
matcher =
    Parser.succeed identity
        |. spaces
        |= item
        |. spaces
        |. Parser.end


item : Parser Matcher
item =
    Parser.succeed Matcher
        |= Parser.variable isVarChar isVarChar Set.empty
        |= (matchers
                |> List.map
                    (\( keyword, matcher ) ->
                        Parser.succeed matcher
                            |. Parser.keyword keyword
                    )
                |> Parser.oneOf
           )
        |= string '"'


spaces : Parser ()
spaces =
    ignore zeroOrMore (\char -> char == ' ' || char == '\t')


string : Char -> Parser String
string separator =
    Parser.succeed identity
        |. Parser.symbol (String.fromChar separator)
        |= stringContents separator
        |. Parser.symbol (String.fromChar separator)


stringContents : Char -> Parser String
stringContents separator =
    Parser.oneOf
        [ Parser.succeed (++)
            |= keepOne (\char -> char == '\\')
            |= keepOne (\char -> True)
        , Parser.keep Parser.oneOrMore (\char -> char /= separator && char /= '\\')
        ]
        |> Parser.repeat Parser.oneOrMore
        |> Parser.map (String.join "")


isVarChar : Char -> Bool
isVarChar char =
    Char.isLower char
        || Char.isUpper char
        || (char == '_')
        || Char.isDigit char


keepOne : (Char -> Bool) -> Parser String
keepOne =
    Parser.keep (Parser.Exactly 1)
