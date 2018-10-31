module Views.Shared.AlertListCompact exposing (view)

import Alerts.Types exposing (Alert)
import Html exposing (Html, ol)
import Html.Attributes exposing (class)
import Views.Shared.AlertCompact
import Views.Shared.Types exposing (Msg(..))


view : Maybe String -> List Alert -> Html Msg
view maybeActiveId alerts =
    List.map (Views.Shared.AlertCompact.view maybeActiveId) alerts
        |> ol [ class "list pa0 w-100" ]
