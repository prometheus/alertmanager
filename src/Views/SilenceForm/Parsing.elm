module Views.SilenceForm.Parsing exposing (silenceFormNewParser, silenceFormEditParser)

import UrlParser exposing (Parser, s, (</>), string, oneOf, map)


silenceFormNewParser : Parser a a
silenceFormNewParser =
    s "silences" </> s "new"


silenceFormEditParser : Parser (String -> a) a
silenceFormEditParser =
    s "silences" </> string </> s "edit"
