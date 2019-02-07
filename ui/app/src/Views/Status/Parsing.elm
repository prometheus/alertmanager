module Views.Status.Parsing exposing (statusParser)

import Url.Parser exposing (Parser, s)


statusParser : Parser a a
statusParser =
    s "status"
