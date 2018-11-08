module Views.SilenceView.Views exposing (view)

import Alerts.Types exposing (Alert)
import Data.Silence exposing (Silence)
import Data.SilenceStatus
import Data.Silences exposing (Silences)
import Html exposing (Html, b, button, div, h1, h2, h3, label, p, span, text)
import Html.Attributes exposing (class, href)
import Html.Events exposing (onClick)
import Silences.Types exposing (stateToString)
import Types exposing (Msg(..))
import Utils.Date exposing (dateTimeFormat)
import Utils.List
import Utils.Types exposing (ApiData(..))
import Utils.Views exposing (error, loading)
import Views.Shared.Dialog as Dialog
import Views.Shared.SilencePreview
import Views.Shared.Types as SharedTypes
import Views.SilenceList.SilenceView exposing (editButton)
import Views.SilenceList.Types exposing (SilenceListMsg(..))
import Views.SilenceView.Types as SilenceViewTypes exposing (Model)


view : Model -> Html Msg
view { silence, alerts, activeAlertId, showConfirmationDialog } =
    case silence of
        Success sil ->
            if showConfirmationDialog then
                viewSilence activeAlertId alerts sil True

            else
                viewSilence activeAlertId alerts sil False

        Initial ->
            loading

        Loading ->
            loading

        Failure msg ->
            error msg


viewSilence : Maybe String -> ApiData (List Alert) -> Silence -> Bool -> Html Msg
viewSilence activeAlertId alerts silence showPromptDialog =
    let
        affectedAlerts =
            Views.Shared.SilencePreview.view activeAlertId alerts
                |> Html.map (\msg -> MsgForSilenceView (SilenceViewTypes.SetActiveAlert msg))
    in
    div []
        [ h1 []
            [ text "Silence"
            , span
                [ class "ml-3" ]
                [ editButton silence
                , expireButton silence False
                ]
            ]
        , formGroup "ID" <| text <| Maybe.withDefault "" silence.id
        , formGroup "Starts at" <| text <| dateTimeFormat silence.startsAt
        , formGroup "Ends at" <| text <| dateTimeFormat silence.endsAt
        , formGroup "Updated at" <| text <| Maybe.withDefault "" <| Maybe.map dateTimeFormat silence.updatedAt
        , formGroup "Created by" <| text <| silence.createdBy
        , formGroup "Comment" <| text silence.comment
        , formGroup "State" <| text <| Maybe.withDefault "" <| Maybe.map (.state >> stateToString) silence.status
        , formGroup "Matchers" <|
            div [] <|
                List.map (Utils.List.mstring >> Utils.Views.labelButton Nothing) silence.matchers
        , affectedAlerts
        , Dialog.view
            (if showPromptDialog then
                Just (confirmSilenceDeleteView silence True)

             else
                Nothing
            )
        ]


confirmSilenceDeleteView : Silence -> Bool -> Dialog.Config Msg
confirmSilenceDeleteView silence refresh =
    -- TODO: silence.id should never be Nothing in this context. How to better handle this?
    { onClose = MsgForSilenceView (SilenceViewTypes.Reload <| Maybe.withDefault "" silence.id)
    , title = "Expire Silence"
    , body = text "Are you sure you want to expire this silence?"
    , footer =
        button
            [ class "btn btn-primary"
            , onClick (MsgForSilenceList (DestroySilence silence refresh))
            ]
            [ text "Confirm" ]
    }


formGroup : String -> Html Msg -> Html Msg
formGroup key content =
    div [ class "form-group row" ]
        [ label [ class "col-2 col-form-label" ] [ b [] [ text key ] ]
        , div [ class "col-10 d-flex align-items-center" ]
            [ content
            ]
        ]


expireButton : Silence -> Bool -> Html Msg
expireButton silence refresh =
    -- TODO: .status should never be nothing, how can we handle this better?
    case silence.status of
        Just { state } ->
            case state of
                Data.SilenceStatus.Expired ->
                    text ""

                Data.SilenceStatus.Active ->
                    button
                        [ class "btn btn-outline-danger border-0"
                        , onClick (MsgForSilenceView (SilenceViewTypes.ConfirmDestroySilence silence refresh))
                        ]
                        [ text "Expire"
                        ]

                Data.SilenceStatus.Pending ->
                    button
                        [ class "btn btn-outline-danger border-0"
                        , onClick (MsgForSilenceView (SilenceViewTypes.ConfirmDestroySilence silence refresh))
                        ]
                        [ text "Delete"
                        ]

        Nothing ->
            text ""
