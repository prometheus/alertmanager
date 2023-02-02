module Views.Settings.Parsing exposing (settingsViewParser)

import Url.Parser exposing (Parser, s)


settingsViewParser : Parser a a
settingsViewParser =
    s "settings"
