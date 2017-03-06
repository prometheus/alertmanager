module Views exposing (..)

import Html exposing (Html, text)
import Types exposing (Msg, Model, Route(SilencesRoute, AlertsRoute))
import Utils.Types exposing (ApiResponse(..))
import Utils.Views exposing (error, loading, notFoundView)
import Translators exposing (alertTranslator, silenceTranslator)
import Silences.Views
import Alerts.Views


view : Model -> Html Msg
view model =
    case model.route of
        AlertsRoute route ->
            case model.alertGroups of
                Success alertGroups ->
                    Html.map alertTranslator (Alerts.Views.view route alertGroups model.filter (text ""))

                Loading ->
                    loading

                Failure msg ->
                    Html.map alertTranslator (Alerts.Views.view route [] model.filter (error msg))

        SilencesRoute route ->
            Html.map silenceTranslator (Silences.Views.view route model.silences model.silence model.currentTime model.filter)

        _ ->
            notFoundView model
