module Views.SilenceForm.Parsing exposing (silenceFormNewParser, silenceFormEditParser)

import UrlParser exposing (Parser, s, (</>), (<?>), string, stringParam, oneOf, map)


silenceFormNewParser : Parser (Bool -> a) a
silenceFormNewParser =
    s "silences"
        </> s "new"
        <?> stringParam "keep"
        |> map (Maybe.map (always True) >> Maybe.withDefault False)


silenceFormEditParser : Parser (String -> a) a
silenceFormEditParser =
    s "silences" </> string </> s "edit"
