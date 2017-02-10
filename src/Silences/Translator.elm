module Silences.Translator exposing (translator)

import Silences.Types exposing (Msg(..), SilencesMsg)


type alias TranslationDictionary msg =
    { onFormMsg : SilencesMsg -> msg
    }


translator : TranslationDictionary parentMsg -> Msg -> parentMsg
translator { onFormMsg } msg =
    case msg of
        ForSelf internal ->
            onFormMsg internal
