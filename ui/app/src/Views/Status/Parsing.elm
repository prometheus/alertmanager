module Views.Status.Parsing exposing (statusParser)

import UrlParser exposing (Parser, s)


statusParser : Parser a a
statusParser =
    s "status"
