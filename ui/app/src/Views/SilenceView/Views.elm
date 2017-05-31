module Views.SilenceView.Views exposing (view)

import Silences.Types exposing (Silence, stateToString)
import Alerts.Types exposing (Alert)
import Html exposing (Html, div, h2, p, text, label, b, h1)
import Html.Attributes exposing (class)
import Utils.Types exposing (ApiData(Initial, Success, Loading, Failure))
import Utils.Views exposing (loading, error)
import Views.Shared.SilencePreview
import Views.SilenceView.Types exposing (Model, SilenceViewMsg)
import Utils.Date exposing (dateTimeFormat)
import Utils.List


view : Model -> Html SilenceViewMsg
view { silence, alerts } =
    case silence of
        Success sil ->
            viewSilence alerts sil

        Initial ->
            loading

        Loading ->
            loading

        Failure msg ->
            error msg


viewSilence : ApiData (List Alert) -> Silence -> Html SilenceViewMsg
viewSilence alerts silence =
    div []
        [ h1 [] [ text "Silence" ]
        , formGroup "ID" <| text silence.id
        , formGroup "Starts at" <| text <| dateTimeFormat silence.startsAt
        , formGroup "Ends at" <| text <| dateTimeFormat silence.endsAt
        , formGroup "Updated at" <| text <| dateTimeFormat silence.updatedAt
        , formGroup "Created by" <| text silence.createdBy
        , formGroup "Comment" <| text silence.comment
        , formGroup "State" <| text <| stateToString silence.status.state
        , formGroup "Matchers" <|
            div [] <|
                List.map (Utils.List.mstring >> Utils.Views.labelButton Nothing) silence.matchers
        , formGroup "Affected alerts" <| Views.Shared.SilencePreview.view alerts
        ]


formGroup : String -> Html SilenceViewMsg -> Html SilenceViewMsg
formGroup key content =
    div [ class "form-group row" ]
        [ label [ class "col-2 col-form-label" ] [ b [] [ text key ] ]
        , div [ class "col-10 d-flex align-items-center" ]
            [ content
            ]
        ]
