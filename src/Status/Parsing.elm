module Status.Parsing exposing (statusParser)

import UrlParser exposing (Parser, s, string, (</>))

statusParser : Parser a a
statusParser =
    s "status"
