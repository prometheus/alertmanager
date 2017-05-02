module Views.Silence.Types exposing (SilenceMsg(..))

import Silences.Types exposing (Silence, SilenceId)
import Alerts.Types exposing (Alert)
import Utils.Types exposing (ApiData)


type SilenceMsg
    = FetchSilence String
    | SilenceFetched (ApiData Silence)
    | AlertGroupsPreview (ApiData (List Alert))
    | InitSilenceView SilenceId
