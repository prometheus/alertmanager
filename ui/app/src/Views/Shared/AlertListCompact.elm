module Views.Shared.AlertListCompact exposing (view)

import Data.Alerts exposing (Alerts)
import Html exposing (Html, ol)
import Html.Attributes exposing (class)
import Views.Shared.AlertCompact


view : Alerts -> Html msg
view alerts =
    List.map Views.Shared.AlertCompact.view alerts
        |> ol [ class "list pa0 w-100" ]
