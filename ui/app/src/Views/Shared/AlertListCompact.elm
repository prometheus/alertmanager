module Views.Shared.AlertListCompact exposing (view)

import Alerts.Types exposing (Alert)
import Html exposing (Html, ol)
import Html.Attributes exposing (class)
import Views.Shared.AlertCompact


view : List Alert -> Html msg
view alerts =
    List.map Views.Shared.AlertCompact.view alerts
        |> ol [ class "list pa0 w-100" ]
