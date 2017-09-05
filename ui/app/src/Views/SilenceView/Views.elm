module Views.SilenceView.Views exposing (view)

import Silences.Types exposing (Silence, stateToString)
import Alerts.Types exposing (Alert)
import Html exposing (Html, div, h2, p, text, label, b, h1, span)
import Html.Attributes exposing (class, href)
import Html.Events exposing (onClick)
import Types exposing (Msg)
import Views.SilenceForm.Types exposing (SilenceFormMsg(NewSilenceFromMatchers))
import Utils.Types exposing (ApiData(Initial, Success, Loading, Failure))
import Utils.Views exposing (loading, error)
import Views.Shared.SilencePreview
import Views.SilenceView.Types exposing (Model)
import Utils.Date exposing (dateTimeFormat)
import Utils.List
import Views.SilenceList.SilenceView exposing (deleteButton, editButton)


view : Model -> Html Msg
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


viewSilence : ApiData (List Alert) -> Silence -> Html Msg
viewSilence alerts silence =
    div []
        [ h1 []
            [ text "Silence"
            , span
                [ class "ml-3" ]
                [ editButton silence
                , deleteButton silence True
                ]
            ]
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


formGroup : String -> Html Msg -> Html Msg
formGroup key content =
    div [ class "form-group row" ]
        [ label [ class "col-2 col-form-label" ] [ b [] [ text key ] ]
        , div [ class "col-10 d-flex align-items-center" ]
            [ content
            ]
        ]
