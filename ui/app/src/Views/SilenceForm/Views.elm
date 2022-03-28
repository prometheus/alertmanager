module Views.SilenceForm.Views exposing (view)

import Data.GettableAlert exposing (GettableAlert)
import Html exposing (Html, button, div, h1, i, input, label, strong, text)
import Html.Attributes exposing (class, style)
import Html.Events exposing (onClick)
import Utils.DateTimePicker.Views exposing (viewDateTimePicker)
import Utils.Filter exposing (SilenceFormGetParams)
import Utils.FormValidation exposing (ValidatedField, ValidationState(..))
import Utils.Types exposing (ApiData)
import Utils.Views exposing (loading, validatedField, validatedTextareaField)
import Views.FilterBar.Types as FilterBar
import Views.FilterBar.Views as FilterBar
import Views.Shared.SilencePreview
import Views.SilenceForm.Types exposing (Model, SilenceForm, SilenceFormFieldMsg(..), SilenceFormMsg(..))


view : Maybe String -> SilenceFormGetParams -> String -> Model -> Html SilenceFormMsg
view maybeId silenceFormGetParams defaultCreator { form, filterBar, filterBarValid, silenceId, alerts, activeAlertId } =
    let
        ( title, resetClick ) =
            case maybeId of
                Just silenceId_ ->
                    ( "Edit Silence", FetchSilence silenceId_ )

                Nothing ->
                    ( "New Silence", NewSilenceFromMatchersAndComment defaultCreator silenceFormGetParams )
    in
    div []
        [ h1 [] [ text title ]
        , timeInput form.startsAt form.endsAt form.duration
        , matchersInput filterBarValid filterBar
        , validatedField input
            "Creator"
            inputSectionPadding
            (UpdateCreatedBy >> UpdateField)
            (ValidateCreatedBy |> UpdateField)
            form.createdBy
        , validatedTextareaField
            "Comment"
            inputSectionPadding
            (UpdateComment >> UpdateField)
            (ValidateComment |> UpdateField)
            form.comment
        , div [ class inputSectionPadding ]
            [ informationBlock activeAlertId silenceId alerts
            , silenceActionButtons maybeId resetClick
            ]
        , dateTimePickerDialog form
        ]


dateTimePickerDialog : SilenceForm -> Html SilenceFormMsg
dateTimePickerDialog form =
    if form.viewDateTimePicker then
        div []
            [ div [ class "modal fade show", style "display" "block" ]
                [ div [ class "modal-dialog modal-dialog-centered" ]
                    [ div [ class "modal-content" ]
                        [ div [ class "modal-header" ]
                            [ button
                                [ class "close ml-auto"
                                , onClick (CloseDateTimePicker |> UpdateField)
                                ]
                                [ text "x" ]
                            ]
                        , div [ class "modal-body" ]
                            [ viewDateTimePicker form.dateTimePicker |> Html.map UpdateDateTimePicker ]
                        , div [ class "modal-footer" ]
                            [ button
                                [ class "ml-2 btn btn-outline-success mr-auto"
                                , onClick (CloseDateTimePicker |> UpdateField)
                                ]
                                [ text "Cancel" ]
                            , button
                                [ class "ml-2 btn btn-primary"
                                , onClick (UpdateTimesFromPicker |> UpdateField)
                                ]
                                [ text "Set Date/Time" ]
                            ]
                        ]
                    ]
                ]
            , div [ class "modal-backdrop fade show" ] []
            ]

    else
        div [ style "clip" "rect(0,0,0,0)", style "position" "fixed" ]
            [ div [ class "modal fade" ] []
            , div [ class "modal-backdrop fade" ] []
            ]


inputSectionPadding : String
inputSectionPadding =
    "mt-5"


timeInput : ValidatedField -> ValidatedField -> ValidatedField -> Html SilenceFormMsg
timeInput startsAt endsAt duration =
    div [ class <| "row " ++ inputSectionPadding ]
        [ validatedField input
            "Start"
            "col-lg-4 col-6"
            (UpdateStartsAt >> UpdateField)
            (ValidateTime |> UpdateField)
            startsAt
        , validatedField input
            "Duration"
            "col-lg-3 col-6"
            (UpdateDuration >> UpdateField)
            (ValidateTime |> UpdateField)
            duration
        , validatedField input
            "End"
            "col-lg-4 col-6"
            (UpdateEndsAt >> UpdateField)
            (ValidateTime |> UpdateField)
            endsAt
        , div
            [ class "form-group col-lg-1 col-6" ]
            [ label
                []
                [ text "\u{00A0}" ]
            , button
                [ class "form-control btn btn-outline-primary cursor-pointer"
                , onClick (OpenDateTimePicker |> UpdateField)
                ]
                [ i
                    [ class "fa fa-calendar"
                    ]
                    []
                ]
            ]
        ]


matchersInput : Utils.FormValidation.ValidationState -> FilterBar.Model -> Html SilenceFormMsg
matchersInput filterBarValid filterBar =
    let
        errorClass =
            case filterBarValid of
                Invalid _ ->
                    " has-danger"

                _ ->
                    ""
    in
    div [ class (inputSectionPadding ++ errorClass) ]
        [ label [ Html.Attributes.for "filter-bar-matcher" ]
            [ strong [] [ text "Matchers " ]
            , text "Alerts affected by this silence"
            ]
        , FilterBar.view { showSilenceButton = False } filterBar |> Html.map MsgForFilterBar
        , case filterBarValid of
            Invalid error ->
                div [ class "form-control-feedback" ] [ text error ]

            _ ->
                text ""
        ]


informationBlock : Maybe String -> ApiData String -> ApiData (List GettableAlert) -> Html SilenceFormMsg
informationBlock activeAlertId silence alerts =
    case silence of
        Utils.Types.Success _ ->
            text ""

        Utils.Types.Initial ->
            Views.Shared.SilencePreview.view activeAlertId alerts
                |> Html.map SetActiveAlert

        Utils.Types.Failure error ->
            Utils.Views.error error

        Utils.Types.Loading ->
            loading


silenceActionButtons : Maybe String -> SilenceFormMsg -> Html SilenceFormMsg
silenceActionButtons maybeId resetClick =
    div [ class ("mb-4 " ++ inputSectionPadding) ]
        [ previewSilenceBtn
        , createSilenceBtn maybeId
        , button
            [ class "ml-2 btn btn-danger", onClick resetClick ]
            [ text "Reset" ]
        ]


createSilenceBtn : Maybe String -> Html SilenceFormMsg
createSilenceBtn maybeId =
    let
        btnTxt =
            case maybeId of
                Just _ ->
                    "Update"

                Nothing ->
                    "Create"
    in
    button
        [ class "ml-2 btn btn-primary"
        , onClick CreateSilence
        ]
        [ text btnTxt ]


previewSilenceBtn : Html SilenceFormMsg
previewSilenceBtn =
    button
        [ class "btn btn-outline-success"
        , onClick PreviewSilence
        ]
        [ text "Preview Alerts" ]
