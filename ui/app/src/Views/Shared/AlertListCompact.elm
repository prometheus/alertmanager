module Views.Shared.AlertListCompact exposing (view)

import Alerts.Types exposing (AlertGroup)
import Html exposing (Html, ul)
import Html.Attributes exposing (class)
import Views.Shared.AlertCompact


view : AlertGroup -> Html msg
view { blocks } =
    blocks
        |> List.concatMap .alerts
        |> List.indexedMap Views.Shared.AlertCompact.view
        |> ul [ class "list pa0" ]
