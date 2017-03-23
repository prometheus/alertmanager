module Views.SilenceList.Types exposing (SilenceListMsg(..))

import Silences.Types exposing (Silence)
import Utils.Types exposing (ApiData)


type SilenceListMsg
    = SilenceDestroy (ApiData String)
    | DestroySilence Silence
    | FilterSilences
    | SilencesFetch (ApiData (List Silence))
    | FetchSilences
