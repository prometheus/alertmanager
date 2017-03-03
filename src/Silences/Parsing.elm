module Silences.Parsing exposing (silencesParser)

import Silences.Types exposing (..)
import UrlParser exposing ((</>), (<?>), Parser, int, map, oneOf, parseHash, s, string, stringParam)


silencesParser : Parser (Route -> a) a
silencesParser =
    oneOf
        [ map ShowSilences list
        , map ShowNewSilence new
        , map ShowEditSilence edit
        , map ShowSilence show
        ]


list : Parser (Maybe String -> a) a
list =
    s "silences" <?> stringParam "filter"


new : Parser a a
new =
    s "silences" </> s "new"


show : Parser (String -> a) a
show =
    s "silences" </> string


edit : Parser (String -> a) a
edit =
    s "silences" </> string </> s "edit"
